package netlink

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/jsimonetti/rtnetlink"
	"github.com/jsimonetti/rtnetlink/rtnl"
)

// PrintRoutes печатает все маршруты, полученные с помощью [rtnl].
//
// [rtnl]: https://pkg.go.dev/github.com/jsimonetti/rtnetlink/rtnl
func PrintRoutes() error {
	c, err := rtnl.Dial(nil)
	if err != nil {
		return err
	}
	defer c.Close()
	links, err := c.Conn.Link.List()
	if err != nil {
		return err
	}
	linksMap := make(map[int]string)
	for _, link := range links {
		linksMap[int(link.Index)] = link.Attributes.Name
	}
	messages, err := c.Conn.Route.List()
	if err != nil {
		return err
	}
	for i, rt := range messages {
		ifindex := int(rt.Attributes.OutIface)
		iface, ok := linksMap[ifindex]
		if !ok {
			return fmt.Errorf("failed to get name of interface with index %d: might be multipath: %s", ifindex, errors.ErrUnsupported)
		}
		var dst string
		if rt.Attributes.Dst == nil {
			dst = "default"
		} else {
			dst = fmt.Sprintf("%s/%d", rt.Attributes.Dst.String(), rt.DstLength)
		}
		var gateway string
		if rt.Attributes.Gateway == nil {
			gateway = ""
		} else {
			gateway = fmt.Sprintf("via %s ", rt.Attributes.Gateway.String())
		}
		fmt.Printf("%02d. %s %sdev %s table id %d\n", i, dst, gateway, iface, rt.Table)
	}
	return nil
}

// SetDefaultRoute добавляет или заменяет маршрут по-умолчанию.
func SetDefaultRoute(gateway string) error {
	if strings.Contains(gateway, ",") {
		gwIps := []net.IP{}
		for _, gwString := range strings.Split(gateway, ",") {
			gwIps = append(gwIps, net.ParseIP(gwString))
		}
		return setMultipathDefaultRoute(gwIps)
	}
	c, err := rtnl.Dial(nil)
	if err != nil {
		return err
	}
	defer c.Close()
	gw := net.ParseIP(gateway)
	routeToGw, err := c.RouteGet(gw)
	if err != nil {
		return fmt.Errorf("route lookup to %s failed: %w", gateway, err)
	}
	_, ipNet, _ := net.ParseCIDR("0.0.0.0/0")
	withRoutePriority := func(opts *rtnl.RouteOptions) {
		opts.Attrs.Priority = 50
	}
	if err := c.RouteReplace(routeToGw.Interface, *ipNet, gw, withRoutePriority); err != nil {
		return err
	}
	return nil
}

// Функция setMultipathDefaultRoute добавляет т.н. [multipath route].
//
// [multipath route]: https://codecave.cc/multipath-routing-in-linux-part-1.html
func setMultipathDefaultRoute(gateways []net.IP) error {
	c, err := rtnl.Dial(nil)
	if err != nil {
		return err
	}
	defer c.Close()
	withRoutePriority := func(opts *rtnl.RouteOptions) {
		opts.Attrs.Priority = 50
	}
	gwInterfaceIndexes := make([]int, len(gateways))
	for i, gw := range gateways {
		routeToGw, err := c.RouteGet(gw)
		if err != nil {
			return fmt.Errorf("route lookup to %s failed: %w", gw, err)
		}
		gwInterfaceIndexes[i] = routeToGw.Interface.Index
	}
	withMultipathRoute := func(opts *rtnl.RouteOptions) {
		nextHops := make([]rtnetlink.NextHop, 0, len(gateways))
		for i, gw := range gateways {
			nextHops = append(nextHops, rtnetlink.NextHop{
				Hop: rtnetlink.RTNextHop{
					Length:  16,
					IfIndex: uint32(gwInterfaceIndexes[i]),
				},
				Gateway: gw,
			})
		}
		opts.Attrs.Multipath = nextHops
	}
	noInterface := &net.Interface{
		Index: 0,
	}
	_, allNets, _ := net.ParseCIDR("0.0.0.0/0")
	noDst := net.IPv4(0, 0, 0, 0)
	if err := c.RouteReplace(noInterface, *allNets, noDst, withRoutePriority, withMultipathRoute); err != nil {
		return err
	}
	return nil
}
