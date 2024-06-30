package netlink

import (
	"fmt"
	"net"
	"strings"

	"github.com/jsimonetti/rtnetlink"
	"github.com/jsimonetti/rtnetlink/rtnl"
)

const (
	familyAfInet     = 2
	rtTableMain      = 254
	protoBgp         = 186
	typeUnicast      = 1
	scopeGlobal      = 0
	noInterfaceIndex = 0
	routePriority    = 50
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
		ifName, ok := linksMap[ifindex]
		if !ok {
			tryPrintMultipathRoute(i, linksMap, rt)
			continue
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
		fmt.Printf("%02d. %s %sdev %s table id %d\n", i, dst, gateway, ifName, rt.Table)
	}
	return nil
}

func tryPrintMultipathRoute(i int, linksMap map[int]string, rt rtnetlink.RouteMessage) {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("%02d. ", i))
	if rt.Attributes.Dst == nil {
		sb.WriteString("default ")
	} else {
		sb.WriteString(fmt.Sprintf("%s/%d ", rt.Attributes.Dst.String(), rt.DstLength))
	}
	sb.WriteString(fmt.Sprintf("proto id %d table id %d priority %d\n", rt.Protocol, rt.Table, rt.Attributes.Priority))
	for i, path := range rt.Attributes.Multipath {
		nextHop := path.Gateway.String()
		ifName, ok := linksMap[int(path.Hop.IfIndex)]
		if !ok {
			sb.WriteString(fmt.Sprintf("\tERROR: failed to determine ifName for nextHop %s\n", nextHop))
			fmt.Print(sb.String())
			return
		}
		sb.WriteString(fmt.Sprintf("\tpath %d: via %s dev %s\n", i, nextHop, ifName))
	}
	fmt.Print(sb.String())
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
		opts.Attrs.Priority = routePriority
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
	nextHops := make([]rtnetlink.NextHop, 0, len(gateways))
	for _, gw := range gateways {
		routeToGw, err := c.RouteGet(gw)
		if err != nil {
			return fmt.Errorf("route lookup to %s failed: %w", gw, err)
		}
		nextHops = append(nextHops, rtnetlink.NextHop{
			Hop: rtnetlink.RTNextHop{
				Length:  16,
				IfIndex: uint32(routeToGw.Interface.Index),
			},
			Gateway: gw,
		})
	}
	_, allNets, _ := net.ParseCIDR("0.0.0.0/0")
	dstlen, _ := allNets.Mask.Size()
	routeMessage := &rtnetlink.RouteMessage{
		Family:    familyAfInet,
		Table:     rtTableMain,
		Protocol:  protoBgp,
		Type:      typeUnicast,
		Scope:     scopeGlobal,
		DstLength: uint8(dstlen),
		SrcLength: uint8(0),
		Attributes: rtnetlink.RouteAttributes{
			Dst:       allNets.IP,
			OutIface:  uint32(noInterfaceIndex),
			Priority:  routePriority,
			Multipath: nextHops,
		},
	}
	return c.Conn.Route.Replace(routeMessage)
}
