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

// Tailscale will assign (v4) addresses in the 100.64.0.0/10 range.
// https://tailscale.com/kb/1015/100.x-addresses
var tailnetIPNet = &netlink.Addr{
	IPNet: &net.IPNet{
		IP:   net.IPv4(100, 64, 0, 0),      // 100.64.0.0
		Mask: net.IPv4Mask(255, 192, 0, 0), // CIDR /10
	},
}

// ValidTailnetAddr4 returns whether an IPNet is a valid Tailscale address.
// I.e., whether an address is in the CIDR network 100.64.0.0/10.
func ValidTailnetAddr4(ipnet *net.IPNet) bool {
	return tailnetIPNet.Contains(ipnet.IP.To4())
}

// TailIf is a Tailscale network interface. It stores information about the
// interface as assigned by tailscaled as well as things such as: addresses
// that should be assigned to it; up commands; down commands.
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

// validateNlLink returns whether a nl link is valid for this tailscale
// interface.
func (t *TailIf) validateNlLink(nlLink netlink.Link) bool {
	return nlLink.Attrs().Name == t.Name
}

// IsUp returns whether the Tailscale interface is 'up'. Up is defined as
// having a valid interface with the correct name that has an associated v4
// address in the range 100.64.0.0/10.
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

// SetDown takes the netlink link of a Tailscale interface and removes all
// tnetmgr addresses. It also sequentially executes the ExecDown instructions
// with the ExecShell shell. If one of these commands raises an error it will
// log rather than returning. Note: other parts of the function may return
// error(s).
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
			slog.Warn("ExecDown command failed", "command", cmd, "error", err.Error())
		}
	}

	return nil
}

// SetUp takes the netlink link of a Tailscale interface and adds all tnetmgr
// addresses. It also sequentially executes the ExcecUp instructions with the
// ExecShell shell. If one of these commands raises an error it will log rather
// than returning. Note: other parts of the function may return error(s).
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
			slog.Warn("ExecDown command failed", "command", cmd, "error", err.Error())
		}
	}

	return nil
}

// LinkExists returns whether a network interface exists for this Tailscale
// interface.
func (t *TailIf) LinkExists() bool {
	nlLink, err := t.GetLink()
	return err == nil && nlLink != nil
}

// GetLink returns the netlink link for this Tailscale interface.
func (t *TailIf) GetLink() (netlink.Link, error) {
	return netlink.LinkByName(t.Name)
}

// Sync will ensure that SetUp is called if the interface exists and is up else
// SetDown is called if the interface exists and is down. If the interface does
// not exist then nothing is run. The alternative to calling this function at
// an interval is listening to the netlink socket for address changes; in that
// case this function needs to be called once at the program start to ensure
// everything is in the correct state before listening. See cmd package.
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
