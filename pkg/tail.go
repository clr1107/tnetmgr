package tnetmgr

import (
	"fmt"
	"net"

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
	Name  string
	Addrs []*netlink.Addr
}

func (t *TailIf) SetDown(nlLink netlink.Link) error {
	var err error
	var linkAddrs []netlink.Addr

	if nlLink.Attrs().Name != t.Name {
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

	return nil
}

func (t *TailIf) SetUp(nlLink netlink.Link) error {
	var err error
	var linkAddrs []netlink.Addr

	if nlLink.Attrs().Name != t.Name {
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

	return nil
}
