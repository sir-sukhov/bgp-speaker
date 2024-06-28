package netlink

import (
	"fmt"
	"net"

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
			return fmt.Errorf("failed to get name of interfae with index %d", ifindex)
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
