package speaker

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	api "github.com/osrg/gobgp/v3/api"
	"github.com/osrg/gobgp/v3/pkg/packet/bgp"
	"github.com/osrg/gobgp/v3/pkg/server"
	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v3"
)

const (
	defaultRoute       = "default-route"
	uplinks            = "uplinks"
	defaultRoutePolicy = "only-default-route"
	onlyAnycastIP      = "only-anycast-ip"
	anycastIP          = "anycast-ip"
	global             = "global"
)

type Speaker struct {
	ctx        context.Context
	confitPath string
	logLevel   LogLevel
	logger     *Logger
	config     Config
	s          *server.BgpServer
}

func NewAppCfg(configPath string, logLevel LogLevel) (*Speaker, error) {
	sp := &Speaker{
		confitPath: configPath,
		logLevel:   logLevel,
	}
	sp.logger = NewLogger(sp.logLevel.LrLevel())
	if err := sp.loadConfig(); err != nil {
		return nil, err
	}
	return sp, nil
}

func (sp *Speaker) loadConfig() error {
	configBytes, err := os.ReadFile(sp.confitPath)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(configBytes, &sp.config)
}

func (sp *Speaker) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	sp.ctx = ctx
	defer stop()

	sp.s = server.NewBgpServer(server.GrpcListenAddress("localhost:6061"), server.LoggerOption(sp.logger))
	go sp.s.Serve()
	defer sp.s.Stop()
	if err := sp.startBgp(); err != nil {
		return fmt.Errorf("error starting bgp: %w", err)
	}
	if err := sp.setupPolicies(); err != nil {
		return fmt.Errorf("error creating policies: %w", err)
	}
	if err := sp.addNeighbors(); err != nil {
		return fmt.Errorf("error adding neighbors: %w", err)
	}
	if err := sp.addPath(); err != nil {
		return fmt.Errorf("error advertising anycast route: %w", err)
	}

	<-ctx.Done()
	sp.logger.logger.Info("context is done, shutting down bgp")
	if err := sp.stopBgp(); err != nil {
		return fmt.Errorf("failed to stop bgp server: %w", err)
	}

	return nil
}

func (sp *Speaker) startBgp() error {
	return sp.s.StartBgp(sp.ctx, &api.StartBgpRequest{
		Global: &api.Global{
			Asn:        sp.config.ASN,
			RouterId:   sp.config.AnycastIP,
			ListenPort: -1,
		},
	})
}

func (sp *Speaker) stopBgp() error {
	return sp.s.StopBgp(sp.ctx, &api.StopBgpRequest{})
}

func (sp *Speaker) addNeighbors() error {
	for _, neighbor := range sp.config.Neighbors {
		peer := &api.Peer{
			Conf: &api.PeerConf{
				NeighborAddress: neighbor.Address,
				PeerAsn:         neighbor.ASN,
			},
		}
		if err := sp.s.AddPeer(sp.ctx, &api.AddPeerRequest{Peer: peer}); err != nil {
			return err
		}
	}
	return nil
}

func (sp *Speaker) addPath() error {
	nlri, err := anypb.New(&api.IPAddressPrefix{
		Prefix:    sp.config.AnycastIP,
		PrefixLen: 32,
	})
	if err != nil {
		return fmt.Errorf("error creating network layer reachability information: %s", err)
	}
	a1, _ := anypb.New(&api.OriginAttribute{
		Origin: uint32(bgp.BGP_ORIGIN_ATTR_TYPE_IGP),
	})
	a2, _ := anypb.New(&api.NextHopAttribute{
		// Это локальный маршрут, nexthop выставляем по аналогии с клиентской утилитой "gobgp":
		//   если выполнить пример из презентации по gobgp с импортом локального маршрута в rib:
		//     https://blog.netravnen.com/storage/2019/08/ixbrforum10day3gobgptutorial-161205210258.pdf
		//     "gobgp global rib add -a ipv4 10.0.0.0/24"
		//   то выполнится строка 1658 файла cmd/gobgp/global.go, устанавливающая такой nexthop
		//     https://github.com/osrg/gobgp/blob/dace87570846cc4b4f16e8b25516b22c43888f76/cmd/gobgp/global.go#L1658
		NextHop: "0.0.0.0",
	})
	path := &api.Path{
		Family: &api.Family{Afi: api.Family_AFI_IP, Safi: api.Family_SAFI_UNICAST},
		Nlri:   nlri,
		Pattrs: []*anypb.Any{a1, a2},
	}
	_, err = sp.s.AddPath(sp.ctx, &api.AddPathRequest{Path: path})
	return err
}

// Метод setupPolicies [настраивает политики], чтобы случайно не принять или не отправить ненужное.
//
// [настраивает политики]: https://github.com/osrg/gobgp/blob/master/docs/sources/policy.md
func (sp *Speaker) setupPolicies() error {
	if err := sp.addDefinedSets(); err != nil {
		return fmt.Errorf("addDefinedSets failed: %w", err)
	}
	policyDefaultRoute := sp.createDefaultRoutePolicy()
	if err := sp.addPolicy(policyDefaultRoute); err != nil {
		return err
	}
	policyAnycastIP := sp.createAnycastIPPolicy()
	if err := sp.addPolicy(policyAnycastIP); err != nil {
		return err
	}
	policyImportAnycastIP := sp.createAnycastIPPolicyImport()
	if err := sp.addPolicy(policyImportAnycastIP); err != nil {
		return err
	}
	if err := sp.addPolicyAssignment(&api.PolicyAssignment{
		Name:          global,
		Direction:     api.PolicyDirection_IMPORT,
		Policies:      []*api.Policy{policyDefaultRoute, policyImportAnycastIP},
		DefaultAction: api.RouteAction_REJECT,
	}); err != nil {
		return err
	}
	if err := sp.addPolicyAssignment(&api.PolicyAssignment{
		Name:          global,
		Direction:     api.PolicyDirection_EXPORT,
		Policies:      []*api.Policy{policyAnycastIP},
		DefaultAction: api.RouteAction_REJECT,
	}); err != nil {
		return err
	}
	return nil
}

// Метод addDefinedSets создает в конфигерации BGP несколько объектов [defined-sets]:
//   - объект с именем "defaultRoute" соответствует префиксу, который анонсирует фабрика
//   - объект с именем "anycastIP" соответствует префиксу, который анонсирует gobgp
//   - объект с именем "uplinks" соответствует bgp-пирам
//
// Имена объектов являются константами, на которые еще ссылаются политики.
//
// [defined-sets]: https://github.com/osrg/gobgp/blob/master/docs/sources/policy.md#1-defining-defined-sets
func (sp *Speaker) addDefinedSets() error {
	prefixSetDefaultRoute := &api.DefinedSet{
		DefinedType: api.DefinedType_PREFIX,
		Name:        defaultRoute,
		Prefixes: []*api.Prefix{
			{
				IpPrefix:      "0.0.0.0/0",
				MaskLengthMin: 0,
				MaskLengthMax: 0,
			},
		},
	}
	if err := sp.addDefinedSet(prefixSetDefaultRoute); err != nil {
		return err
	}
	prefixSetAnycastIP := &api.DefinedSet{
		DefinedType: api.DefinedType_PREFIX,
		Name:        anycastIP,
		Prefixes: []*api.Prefix{
			{
				IpPrefix:      fmt.Sprintf("%s/32", sp.config.AnycastIP),
				MaskLengthMin: 32,
				MaskLengthMax: 32,
			},
		},
	}
	if err := sp.addDefinedSet(prefixSetAnycastIP); err != nil {
		return err
	}
	neighbors := []string{}
	for _, n := range sp.config.Neighbors {
		neighbors = append(neighbors, fmt.Sprintf("%s/32", n.Address))
	}
	neighborSet := api.DefinedSet{
		DefinedType: api.DefinedType_NEIGHBOR,
		Name:        uplinks,
		List:        neighbors,
	}
	if err := sp.addDefinedSet(&neighborSet); err != nil {
		return err
	}
	return nil
}

// Метод createDefaultRoutePolicy создает политику, разрешающую "default route".
func (sp *Speaker) createDefaultRoutePolicy() *api.Policy {
	return &api.Policy{
		Name: defaultRoutePolicy,
		Statements: []*api.Statement{
			{
				Name: "allow-default-route",
				Conditions: &api.Conditions{
					PrefixSet: &api.MatchSet{
						Type: api.MatchSet_ANY,
						Name: defaultRoute,
					},
					NeighborSet: &api.MatchSet{
						Type: api.MatchSet_ANY,
						Name: uplinks,
					},
				},
				Actions: &api.Actions{
					RouteAction: api.RouteAction_ACCEPT,
				},
			},
		},
	}
}

// Метод createAnycastIPPolicy создает политику, разрешающую anycast ip.
func (sp *Speaker) createAnycastIPPolicy() *api.Policy {
	return &api.Policy{
		Name: onlyAnycastIP,
		Statements: []*api.Statement{
			{
				Name: "allow-anycast-ip",
				Conditions: &api.Conditions{
					PrefixSet: &api.MatchSet{
						Type: api.MatchSet_ANY,
						Name: anycastIP,
					},
					NeighborSet: &api.MatchSet{
						Type: api.MatchSet_ANY,
						Name: uplinks,
					},
				},
				Actions: &api.Actions{
					RouteAction: api.RouteAction_ACCEPT,
				},
			},
		},
	}
}

// Метод createAnycastIPPolicy создает политику, разрешающую добавлять в rib anycast ip.
func (sp *Speaker) createAnycastIPPolicyImport() *api.Policy {
	return &api.Policy{
		Name: onlyAnycastIP,
		Statements: []*api.Statement{
			{
				Name: "allow-anycast-ip-igp",
				Conditions: &api.Conditions{
					PrefixSet: &api.MatchSet{
						Type: api.MatchSet_ANY,
						Name: anycastIP,
					},
					RouteType: api.Conditions_ROUTE_TYPE_LOCAL,
				},
				Actions: &api.Actions{
					RouteAction: api.RouteAction_ACCEPT,
				},
			},
		},
	}
}

func (sp *Speaker) addDefinedSet(s *api.DefinedSet) error {
	if err := sp.s.AddDefinedSet(sp.ctx, &api.AddDefinedSetRequest{DefinedSet: s}); err != nil {
		return fmt.Errorf("error creating defined-set \"%s\": %w", s.Name, err)
	}
	return nil
}

func (sp *Speaker) addPolicyAssignment(a *api.PolicyAssignment) error {
	if err := sp.s.AddPolicyAssignment(sp.ctx, &api.AddPolicyAssignmentRequest{Assignment: a}); err != nil {
		return fmt.Errorf("error creating policy assignment \"%s\": %w", a.Name, err)
	}
	return nil
}

func (sp *Speaker) addPolicy(p *api.Policy) error {
	if err := sp.s.AddPolicy(sp.ctx, &api.AddPolicyRequest{Policy: p, ReferExistingStatements: false}); err != nil {
		return fmt.Errorf("failed to add policy \"%s\": %w", p.Name, err)
	}
	return nil
}
