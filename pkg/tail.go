package tnetmgr

import (
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/sagikazarmark/slog-shim"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

var tailnetIPNet = &netlink.Addr{
	IPNet: &net.IPNet{
		IP:   net.IPv4(100, 64, 0, 0),      // 100.64.0.0
		Mask: net.IPv4Mask(255, 192, 0, 0), // CIDR /10
	},
}

func ValidTailnetAddr4(ipnet *net.IPNet) bool {
	return tailnetIPNet.Contains(ipnet.IP.To4())
}

type TailIf struct {
	Name      string
	Addrs     []*netlink.Addr
	ExecShell string
	ExecUp    []string
	ExecDown  []string
}

func (t *TailIf) createShellCommand(cmd string) *exec.Cmd {
	fields := strings.Fields(t.ExecShell)
	fields = append(fields, cmd)

	execCmd := exec.Command(fields[0], fields[1:]...)
	execCmd.Stderr = slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug).Writer()
	execCmd.Stdout = slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug).Writer()

	return execCmd
}

func (t *TailIf) validateNlLink(nlLink netlink.Link) bool {
	return nlLink.Attrs().Name == t.Name
}

func (t *TailIf) IsUp() bool {
	var err error
	var nlLink netlink.Link

	if nlLink, err = t.GetLink(); err != nil {
		return false
	}

	if addr, err := netlink.AddrList(nlLink, unix.AF_INET); err != nil {
		return false
	} else {
		for _, k := range addr {
			if ValidTailnetAddr4(k.IPNet) {
				return true
			}
		}

		return false
	}
}

func (t *TailIf) SetDown(nlLink netlink.Link) error {
	var err error
	var linkAddrs []netlink.Addr

	if !t.validateNlLink(nlLink) {
		return fmt.Errorf("got interface %s expected %s", nlLink.Attrs().Name, t.Name)
	}

	if linkAddrs, err = netlink.AddrList(nlLink, unix.AF_INET); err != nil {
		return err
	}

	for _, k := range t.Addrs {
	search:
		for _, linkAddr := range linkAddrs {
			if k.Equal(linkAddr) {
				if err := netlink.AddrDel(nlLink, &linkAddr); err != nil {
					return err
				} else {
					break search
				}
			}
		}
	}

	for _, cmd := range t.ExecDown {
		cmd := t.createShellCommand(cmd)

		slog.Debug("ExecDown", "command", cmd)

		if err := cmd.Run(); err != nil {
			slog.Info("ExecDown command failed", "command", cmd, "error", err.Error())
		}
	}

	return nil
}

func (t *TailIf) SetUp(nlLink netlink.Link) error {
	var err error
	var linkAddrs []netlink.Addr

	if !t.validateNlLink(nlLink) {
		return fmt.Errorf("got interface %s expected %s", nlLink.Attrs().Name, t.Name)
	}

	if linkAddrs, err = netlink.AddrList(nlLink, unix.AF_INET); err != nil {
		return err
	}

	for _, k := range t.Addrs {
		var exists bool

	search:
		for _, linkAddr := range linkAddrs {
			if k.Equal(linkAddr) {
				exists = true
				break search
			}
		}

		if !exists {
			if err := netlink.AddrAdd(nlLink, k); err != nil {
				return err
			}
		}
	}

	for _, cmd := range t.ExecUp {
		cmd := t.createShellCommand(cmd)

		slog.Debug("ExecUp", "command", cmd)

		if err := cmd.Run(); err != nil {
			slog.Info("ExecDown command failed", "command", cmd, "error", err.Error())
		}
	}

	return nil
}

func (t *TailIf) LinkExists() bool {
	nlLink, err := t.GetLink()
	return err == nil && nlLink != nil
}

func (t *TailIf) GetLink() (netlink.Link, error) {
	return netlink.LinkByName(t.Name)
}

func (t *TailIf) Sync() error {
	if nlink, err := t.GetLink(); err != nil {
		return err
	} else {
		if t.IsUp() {
			return t.SetUp(nlink)
		} else {
			return t.SetDown(nlink)
		}
	}
}
