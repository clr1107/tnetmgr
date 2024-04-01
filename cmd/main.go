package main

import (
	"fmt"

	tnetmgr "github.com/clr1107/tnetmgr/pkg"
	"github.com/vishvananda/netlink"
)

func main() {
	desiredAddr, _ := netlink.ParseAddr("172.24.24.1/10")

	tailIface := tnetmgr.TailIf{
		Name:  "tailscale0",
		Addrs: []*netlink.Addr{desiredAddr},
	}

	ch_link := make(chan netlink.LinkUpdate)
	ch_addr := make(chan netlink.AddrUpdate)

	done := make(chan struct{})

	if err := netlink.LinkSubscribe(ch_link, done); err != nil {
		panic(err)
	}
	if err := netlink.AddrSubscribe(ch_addr, done); err != nil {
		panic(err)
	}

	defer close(done)

	// Handle events
	for {
		select {
		case update := <-ch_addr:
			link, err := netlink.LinkByIndex(int(update.LinkIndex))
			if err != nil {
				fmt.Printf("Error getting link: %v\n", err)
				continue
			}

			if !tnetmgr.ValidTailnetAddr4(&update.LinkAddress) {
				continue
			}

			if update.NewAddr { // new address has been added
				if err := tailIface.SetUp(link); err != nil {
					panic(err)
				}
			} else { // an address has been removed
				if err := tailIface.SetDown(link); err != nil {
					panic(err)
				}
			}

			println()
		}
	}
}
