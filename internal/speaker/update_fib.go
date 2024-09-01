package speaker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jsimonetti/rtnetlink"
	"github.com/mdlayher/netlink"
	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/log"
	"golang.org/x/exp/maps"
)

const (
	UpdateFIBIntervalSeconds = 1
	familyAfInet             = 2
	rtTableMain              = 254
	protoBgp                 = 186
	typeUnicast              = 1
	scopeGlobal              = 0
	defaultPriority          = 170
	getRoute                 = 0x1a
	newRoute                 = 0x18
	deleteRoute              = 0x19
	replaceFlags             = netlink.Request | netlink.Create | netlink.Replace | netlink.Acknowledge
)

func (sp *Speaker) UpdateFIB(ctx context.Context) error {
	c, err := rtnetlink.Dial(nil)
	if err != nil {
		return err
	}
	defer c.Close()
	sp.conn = c
	ticker := time.NewTicker(time.Second * UpdateFIBIntervalSeconds)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			sp.logger.Info(fmt.Sprintf("stop updating FIB: %s", ctx.Err().Error()), nil)
			return sp.cleanupDefaultRoute()
		case <-ticker.C:
			if err := sp.setDefaultRoute(ctx); err != nil {
				sp.logger.Error("error setting default route", log.Fields{"error": err.Error()})
			}
		}
	}
}

func (sp *Speaker) setDefaultRoute(ctx context.Context) error {
	req := api.ListPathRequest{
		TableType: api.TableType_GLOBAL,
		Family: &api.Family{
			Afi:  api.Family_AFI_IP,
			Safi: api.Family_SAFI_UNICAST,
		},
	}
	defaultRoutes := []*api.Destination{}
	filterDefaultRoutes := func(d *api.Destination) {
		if d.Prefix == zeroPrefix {
			defaultRoutes = append(defaultRoutes, d)
		}
	}
	if err := sp.s.ListPath(ctx, &req, filterDefaultRoutes); err != nil {
		return fmt.Errorf("bgp list path error: %w", err)
	}
	if len(defaultRoutes) == 0 {
		return nil
	}
	if len(defaultRoutes) > 1 {
		return fmt.Errorf("unexpeted number of default routes: %w", errors.ErrUnsupported)
	}
	defaultRoute := defaultRoutes[0]
	if len(defaultRoute.Paths) == 1 {
		return sp.setSinglePathRoute(defaultRoute.Paths[0])
	}
	return sp.setMultiPathRoute(defaultRoute.Paths)
}

func (sp *Speaker) cleanupDefaultRoute() error {
	routeMessage := &rtnetlink.RouteMessage{
		Family:   familyAfInet,
		Table:    rtTableMain,
		Protocol: protoBgp,
		Type:     typeUnicast,
		Attributes: rtnetlink.RouteAttributes{
			Priority: sp.linuxRouteMetric,
		},
	}
	oldDefaultRoute, err := sp.getLinuxBGPDefaultRoute()
	if err != nil {
		return fmt.Errorf("cleanupDefaultRoute: failed to lookup default route: %w", err)
	}
	if oldDefaultRoute != nil {
		_, err = sp.conn.Execute(routeMessage, deleteRoute, netlink.Request|netlink.Acknowledge)
		if err != nil {
			return fmt.Errorf("bgp default route cleanup from linux failed: %w", err)
		}
	}
	return nil
}

func (sp *Speaker) setSinglePathRoute(path *api.Path) error {
	newGateway, err := nextHop(path)
	if err != nil {
		return fmt.Errorf("failed to retrieve gateway: %w", err)
	}
	oldDefaultRoute, err := sp.getLinuxBGPDefaultRoute()
	if err != nil {
		return fmt.Errorf("setSinglePathRoute: failed to lookup default route: %w", err)
	}
	if oldDefaultRoute != nil &&
		oldDefaultRoute.Attributes.Gateway.String() == newGateway {
		return nil
	}
	gateway := net.ParseIP(newGateway)
	if gateway.To4() == nil {
		return fmt.Errorf("gateway is not ipv4: %w", errors.ErrUnsupported)
	}
	routeMessage := &rtnetlink.RouteMessage{
		Family:   familyAfInet,
		Table:    rtTableMain,
		Protocol: protoBgp,
		Type:     typeUnicast,
		Attributes: rtnetlink.RouteAttributes{
			Gateway:  gateway,
			Priority: sp.linuxRouteMetric,
		},
	}
	sp.logger.Info("setting linux single path default route", log.Fields{"dst": newGateway})
	_, err = sp.conn.Execute(routeMessage, newRoute, replaceFlags)
	return err
}

func (sp *Speaker) setMultiPathRoute(paths []*api.Path) error {
	newNextHops := map[string]struct{}{}
	for _, path := range paths {
		nextHop, err := nextHop(path)
		if err != nil {
			return fmt.Errorf("failed to retrieve gateway: %w", err)
		}
		newNextHops[nextHop] = struct{}{}
	}
	oldDefaultRoute, err := sp.getLinuxBGPDefaultRoute()
	if err != nil {
		return fmt.Errorf("setMultiPathRoute: failed to lookup default route: %w", err)
	}
	if oldDefaultRoute != nil && oldDefaultRoute.Attributes.Multipath != nil && len(oldDefaultRoute.Attributes.Multipath) == len(newNextHops) {
		routesAreEqual := true
		for _, oldNextHop := range oldDefaultRoute.Attributes.Multipath {
			if _, ok := newNextHops[oldNextHop.Gateway.String()]; !ok {
				routesAreEqual = false
			}
		}
		if routesAreEqual {
			return nil
		}
	}
	nextHops := []rtnetlink.NextHop{}
	for gw := range newNextHops {
		gateway := net.ParseIP(gw)
		if gateway.To4() == nil {
			return fmt.Errorf("gateway is not ipv4: %w", errors.ErrUnsupported)
		}
		nextHops = append(nextHops, rtnetlink.NextHop{
			Gateway: gateway,
		})
	}
	sp.logger.Info("setting linux multi path default route", log.Fields{"dst": maps.Keys(newNextHops)})
	routeMessage := &rtnetlink.RouteMessage{
		Family:   familyAfInet,
		Table:    rtTableMain,
		Protocol: protoBgp,
		Type:     typeUnicast,
		Attributes: rtnetlink.RouteAttributes{
			Priority:  sp.linuxRouteMetric,
			Multipath: nextHops,
		},
	}
	_, err = sp.conn.Execute(routeMessage, newRoute, replaceFlags)
	return err
}

func (sp *Speaker) getLinuxBGPDefaultRoute() (*rtnetlink.RouteMessage, error) {
	msgs, err := sp.conn.Execute(&rtnetlink.RouteMessage{}, getRoute, netlink.Request|netlink.Dump)
	if err != nil {
		return nil, fmt.Errorf("failed to get table of routes: %w", err)
	}
	for i := range msgs {
		route, ok := msgs[i].(*rtnetlink.RouteMessage)
		if !ok {
			return nil, fmt.Errorf("unexpected rtnetlink message: %w", errors.ErrUnsupported)
		}
		if sp.linuxRouteIsMine(route) {
			return route, nil
		}
	}
	return nil, nil
}

func (sp *Speaker) linuxRouteIsMine(route *rtnetlink.RouteMessage) bool {
	return route.Protocol == protoBgp &&
		route.DstLength == 0 &&
		route.Table == rtTableMain &&
		route.Family == familyAfInet &&
		route.Type == typeUnicast &&
		route.Scope == scopeGlobal &&
		route.Attributes.Priority == sp.linuxRouteMetric
}

func nextHop(path *api.Path) (string, error) {
	nextHopAttr := new(api.NextHopAttribute)
	for _, attr := range path.Pattrs {
		if attr.MessageIs(nextHopAttr) {
			if err := attr.UnmarshalTo(nextHopAttr); err != nil {
				return "", err
			}
			return nextHopAttr.NextHop, nil
		}
	}
	return "", fmt.Errorf("faild to extract next hop from gobgp api.Path")
}
