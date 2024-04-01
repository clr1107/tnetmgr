package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	tnetmgr "github.com/clr1107/tnetmgr/pkg"
	"github.com/spf13/viper"
	"github.com/vishvananda/netlink"
)

// LevelDebug Level = -4
// LevelInfo  Level = 0
// LevelWarn  Level = 4
// LevelError Level = 8

var logLevelNames = map[string]slog.Level{
	"DEBUG": slog.LevelDebug,
	"INFO":  slog.LevelInfo,
	"WARN":  slog.LevelWarn,
	"ERROR": slog.LevelError,
}

func initSlog() {
	level := viper.GetString("loglevel")

	if logLevel, ok := logLevelNames[level]; !ok {
		panic(fmt.Errorf("invalid log level given: %s, expected from DEBUG, INFO, WARN, ERROR", level))
	} else {
		slog.SetLogLoggerLevel(logLevel)
	}
}

func setEnvDefaults() {
	viper.SetEnvPrefix("TNETMGR")

	viper.BindEnv("CONFIG_DIR")
	viper.BindEnv("LOGLEVEL")

	viper.SetDefault("CONFIG_DIR", "/etc/tnetmgr")
	viper.SetDefault("LOGLEVEL", "INFO")
}

func setConfigDefaults() {
	viper.SetDefault("Iface", "tailscale0")
	viper.SetDefault("ExecShell", "/usr/bin/bash -c")
	viper.SetDefault("ExecUp", []string{})
	viper.SetDefault("ExecDown", []string{})
}

type config struct {
	Iface     string
	Addrs     []string
	ExecShell string
	ExecUp    []string
	ExecDown  []string
}

var conf *config

func init() {
	setEnvDefaults()
	setConfigDefaults()

	initSlog()
	slog.Debug("initialising")

	viper.SetConfigName("config")
	viper.AddConfigPath(viper.GetString("config_dir"))

	if err := viper.ReadInConfig(); err != nil {
		slog.Error(fmt.Errorf("could not read config file: %w", err).Error())
		os.Exit(1)
	}

	conf = &config{}
	if err := viper.Unmarshal(conf); err != nil {
		slog.Error(fmt.Errorf("could not unmarshal config file: %w", err).Error())
		os.Exit(1)
	}
}

type instance struct {
	iface     string
	addrs     []*netlink.Addr
	execShell string
	execUp    []string
	execDown  []string
}

func parseConfig(conf *config) (*instance, error) {
	var ret instance

	if conf.Iface = strings.TrimSpace(conf.Iface); conf.Iface == "" {
		return nil, errors.New("parsing config iface: iface cannot be empty")
	}
	ret.iface = conf.Iface

	addrs := make([]*netlink.Addr, len(conf.Addrs))
	for i, s := range conf.Addrs {
		if addr, err := netlink.ParseAddr(s); err != nil {
			return nil, fmt.Errorf("parsing config addrs: %w", err)
		} else {
			addrs[i] = addr
		}
	}
	ret.addrs = addrs

	ret.execShell = conf.ExecShell
	ret.execUp = conf.ExecUp
	ret.execDown = conf.ExecDown

	return &ret, nil
}

func main() {
	// 1. Parse
	// 2. Is the list empty? If so, don't bother with any of this
	// 3. Print all values to stdout
	// 4. Subscribe

	var err error
	var inst *instance

	slog.Debug("parsing configuration")
	if inst, err = parseConfig(conf); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	slog.Info("using interface name", "name", inst.iface)

	if len(inst.addrs) == 0 {
		slog.Info("no addresses to manage")
	} else {
		for _, addr := range inst.addrs {
			slog.Info("managing", "address", addr.String())
		}
	}

	slog.Info("registered ExecUp commands", "len(ExecUp)", len(inst.execUp))
	slog.Info("registered ExecDown commands", "len(ExecDown)", len(inst.execDown))

	tailIface := tnetmgr.TailIf{
		Name:      inst.iface,
		Addrs:     inst.addrs,
		ExecShell: inst.execShell,
		ExecUp:    inst.execUp,
		ExecDown:  inst.execDown,
	}

	if _, err = tailIface.GetLink(); err == nil {
		slog.Debug("Syncing interface")
		if err := tailIface.Sync(); err != nil {
			slog.Error(fmt.Errorf("failed to sync iface %s: %w", tailIface.Name, err).Error())
			os.Exit(1)
		} else {
			slog.Debug("synced", "interface", tailIface.Name)
		}
	} else {
		slog.Warn("could not sync; interface does not exist yet", "interface", tailIface.Name)
	}

	ch := make(chan netlink.AddrUpdate)
	done := make(chan struct{})

	if err := netlink.AddrSubscribe(ch, done); err != nil {
		slog.Error(fmt.Errorf("failed to subscribe to address netlink packets: %w", err).Error())
		os.Exit(1)
	} else {
		slog.Debug("subscribed to netlink address packets")
	}

	defer close(done)

	slog.Info("READY listening to address changes on interface!")

outer:
	for update := range ch {
		slog.Debug("update received")

		nlLink, err := netlink.LinkByIndex(int(update.LinkIndex))
		if err != nil {
			slog.Error(fmt.Errorf("error getting updated netlink link: %w", err).Error())
			os.Exit(1)
		}

		// check if this is an address managed by us
		for _, k := range tailIface.Addrs {
			if update.LinkAddress.IP.Equal(k.IP) {
				slog.Debug("address managed by tnetmgr, skipping", "address", update.LinkAddress.String())
				continue outer
			}
		}

		if !tnetmgr.ValidTailnetAddr4(&update.LinkAddress) {
			slog.Debug("update was not a Tailscale address", "address", update.LinkAddress.String())
			continue outer
		} else {
			slog.Debug("update was a Tailscale address", "address", update.LinkAddress.String())
		}

		if update.NewAddr { // new address has been added
			slog.Debug("update was adding a Tailscale address; setting link up")
			if err := tailIface.SetUp(nlLink); err != nil {
				slog.Error(fmt.Errorf("failed to register link up: %w", err).Error())
				os.Exit(1)
			} else {
				slog.Debug("link set up")
			}
		} else { // an address has been removed
			slog.Debug("update was deleting a Tailscale address; setting link down")
			if err := tailIface.SetDown(nlLink); err != nil {
				slog.Error(fmt.Errorf("failed to register link down: %w", err).Error())
				os.Exit(1)
			} else {
				slog.Debug("link set down")
			}
		}
	}
}
