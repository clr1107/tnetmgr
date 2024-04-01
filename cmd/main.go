package main

import (
	"errors"
	"fmt"
	"strings"

	tnetmgr "github.com/clr1107/tnetmgr/pkg"
	"github.com/spf13/viper"
	"github.com/vishvananda/netlink"
)

func setConfigDefaults() {
	viper.SetDefault("config_dir", "/etc/tnetmgr")
	viper.SetDefault("Iface", "tailscale0")
}

type config struct {
	Iface string
	Addrs []string
}

var conf *config

func init() {
	viper.SetEnvPrefix("TNETMGR")
	viper.BindEnv("CONFIG_DIR")

	setConfigDefaults()

	viper.SetConfigName("config")
	viper.AddConfigPath(viper.GetString("config_dir"))

	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("could not read config file: %w", err))
	}

	conf = &config{}
	if err := viper.Unmarshal(conf); err != nil {
		panic(fmt.Errorf("could not unmarshal config file: %w", err))
	}
}

type instance struct {
	iface string
	addrs []*netlink.Addr
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

	return &ret, nil
}

func main() {
	// 1. Parse
	// 2. Is the list empty? If so, don't bother with any of this
	// 3. Print all values to stdout
	// 4. Subscribe

	var err error
	var inst *instance

	if inst, err = parseConfig(conf); err != nil {
		panic(err)
	}

	fmt.Printf("interface %s\n", inst.iface)

	if len(inst.addrs) == 0 {
		fmt.Printf("no addresses to listen to")
		return
	} else {
		for _, addr := range inst.addrs {
			fmt.Printf("\taddress %s\n", addr.String())
		}
	}

	tailIface := tnetmgr.TailIf{
		Name:  inst.iface,
		Addrs: inst.addrs,
	}

	if _, err = tailIface.GetLink(); err == nil {
		if err := tailIface.Sync(); err != nil {
			panic(fmt.Errorf("failed to sync iface %s: %w", tailIface.Name, err))
		}
	}

	ch := make(chan netlink.AddrUpdate)
	done := make(chan struct{})

	if err := netlink.AddrSubscribe(ch, done); err != nil {
		panic(fmt.Errorf("failed to subscribe to address netlink packets: %w", err))
	}

	defer close(done)

	for update := range ch {
		nlLink, err := netlink.LinkByIndex(int(update.LinkIndex))
		if err != nil {
			panic(fmt.Errorf("error getting updated netlink link: %w", err))
		}

		if !tnetmgr.ValidTailnetAddr4(&update.LinkAddress) {
			continue
		}

		if update.NewAddr { // new address has been added
			if err := tailIface.SetUp(nlLink); err != nil {
				panic(fmt.Errorf("failed to register link up: %w", err))
			}
		} else { // an address has been removed
			if err := tailIface.SetDown(nlLink); err != nil {
				panic(fmt.Errorf("failed to register link down: %w", err))
			}
		}
	}
}
