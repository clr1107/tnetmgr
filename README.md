# tnetmgr

A tailscale interface manager. This daemon listens for a Tailscale interface to come up (not the link itself, but for an address in `100.64.0.0/10`) and will add other IP addresses as well as execute arbitrary commands.

## Use case

It's a bit of a bastardisation of how Tailscale would like people to use the product, in my opinion, but it has its usecases. I use it to add an extra address to the tailscale0 interface, so that I can use that address when it is used as a subnet router. I also use it to control the tailscale0 interface with iptables. My example configuration is below.

```yaml
Iface: tailscale0
Addrs:
  - 172.24.24.1/32
ExecUp:
  - iptables -D ts-input -i tailscale0 -j ACCEPT
  - iptables -N ts-fw
  - iptables -A ts-input -i tailscale0 -j ts-fw
```

This configuration adds the address `172.24.24.1/32` to the interface tailscale0 whenever it is connected to Tailscale. I can then use this, for example, with the `--advertise-routes` option.

The iptables commands in ExecUp remove the default accept all rule added by Tailscale every time the interface comes up and replaces it with a new table, called `ts-fw`. This can then be configured. E.g.,

```bash
-I ts-fw -i tailscale0 -p tcp --dport 22 -j ACCEPT
-A ts-fw -i tailscale0 -j DROP
```

This will allow me to connect to SSH via Tailscale and drop all other connections.