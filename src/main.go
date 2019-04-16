package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"github.com/astaxie/beego/logs"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	alsV22 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	alsFilterV21 "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	alsV2 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	"github.com/envoyproxy/go-control-plane/pkg/cache"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
	"github.com/envoyproxy/go-control-plane/pkg/util"
	aLogs "github.com/salrashid123/envoy_control/src/pkg/accesslogs"
	"github.com/salrashid123/envoy_control/src/pkg/callback"
	"github.com/salrashid123/envoy_control/src/pkg/logger"
	"github.com/salrashid123/envoy_control/src/pkg/nodehash"
	"google.golang.org/grpc"
	"net"
	"os"
	"sync/atomic"
	"time"
)

var (
	debug       bool
	onlyLogging bool
	localhost   = "0.0.0.0"
	port        uint
	alsPort     uint
	mode        string
	version     int32
	config      cache.SnapshotCache
)

const (
	XdsCluster               = "xds_cluster"
	Ads                      = "ads"
	Xds                      = "xds"
	Rest                     = "rest"
	gRpcMaxConcurrentStreams = 1000000
)

func init() {
	flag.BoolVar(&debug, "debug", true, "Use debug logging")
	flag.BoolVar(&onlyLogging, "onlyLogging", false, "Only demo AccessLogging Service")
	flag.UintVar(&port, "port", 18000, "Management server port")
	flag.UintVar(&alsPort, "als", 18090, "Access log server port")
	flag.StringVar(&mode, "ads", Ads, "Management server type (ads, xds, rest)")
}

func main() {
	flag.Parse()
	if debug {
		logs.SetLevel(logs.LevelDebug)
	}

	logs.Debug("Starting control plane")

	signal := make(chan struct{})
	cb := callback.NewCallbacks(signal)
	config = cache.NewSnapshotCache(mode == Ads, nodehash.NodeHash{}, logger.Logger{})

	srv := xds.NewServer(config, cb)
	als := &aLogs.AccessLogService{}

	ctx := context.Background()
	go RunAccessLogServer(ctx, als, alsPort)

	if onlyLogging {
		cc := make(chan struct{})
		<-cc
		os.Exit(0)
	}

	// start the xDS server
	go RunManagementServer(ctx, srv, port)

	<-signal

	als.Dump(func(s string) { logs.Info(s) })
	cb.Report()

	for {
		atomic.AddInt32(&version, 1)
		nodeId := config.GetStatusKeys()[0]

		// cluster + hosts

		var clusterName = "mock_server"
		var remoteHost = "10.67.52.249"
		logs.Debug(">>>>>>>>>>>>>>>>>>> creating cluster " + clusterName)

		host := &core.Address{Address: &core.Address_SocketAddress{
			SocketAddress: &core.SocketAddress{
				Address: remoteHost,
				//Protocol: core.TCP,
				PortSpecifier: &core.SocketAddress_PortValue{
					PortValue: uint32(18010),
				},
			},
		}}

		c := []cache.Resource{
			&v2.Cluster{
				Name:           clusterName,
				ConnectTimeout: 2 * time.Second,
				Hosts:          []*core.Address{host},
			},
		}

		// listener

		var listenerName = "listener_0"
		var targetRegex = "/"
		var virtualHostName = "backend"
		var routeConfigName = "local_route"

		logs.Debug(">>>>>>>>>>>>>>>>>>> creating listener " + listenerName)

		v := route.VirtualHost{
			Name:    virtualHostName,
			Domains: []string{"*"},

			Routes: []route.Route{{
				Match: route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: targetRegex,
					},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						ClusterSpecifier: &route.RouteAction_Cluster{
							Cluster: clusterName,
						},
					},
				},
			}}}

		accessConf := &alsV22.HttpGrpcAccessLogConfig{
			CommonConfig: &alsV22.CommonGrpcAccessLogConfig{
				LogName: "accesslog",
				GrpcService: &core.GrpcService{
					TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
							ClusterName: "accesslog_cluster",
						},
					},
				},
			},
		}

		accessConfig, err := util.MessageToStruct(accessConf)
		if err != nil {
			panic(err)
		}

		manager := &hcm.HttpConnectionManager{
			CodecType:  hcm.AUTO,
			StatPrefix: "ingress_http",
			RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
				RouteConfig: &v2.RouteConfiguration{
					Name:         routeConfigName,
					VirtualHosts: []route.VirtualHost{v},
				},
			},
			HttpFilters: []*hcm.HttpFilter{{
				Name: util.Router,
			}},
			AccessLog: []*alsFilterV21.AccessLog{{
				Name: util.HTTPGRPCAccessLog,
				ConfigType: &alsFilterV21.AccessLog_Config{
					Config: accessConfig,
				},
			},
			},
		}

		filterConfig, err := util.MessageToStruct(manager)
		if err != nil {
			panic(err)
		}

		var l = []cache.Resource{
			&v2.Listener{
				Name: listenerName,
				Address: core.Address{
					Address: &core.Address_SocketAddress{
						SocketAddress: &core.SocketAddress{
							Protocol: core.TCP,
							Address:  localhost,
							PortSpecifier: &core.SocketAddress_PortValue{
								PortValue: 10000,
							},
						},
					},
				},
				FilterChains: []listener.FilterChain{{
					Filters: []listener.Filter{{
						Name: util.HTTPConnectionManager,
						ConfigType: &listener.Filter_Config{
							Config: filterConfig,
						},
					}},
				}},
			}}

		logs.Debug(">>>>>>>>>>>>>>>>>>> creating snapshot Version " + fmt.Sprint(version))
		snap := cache.NewSnapshot(fmt.Sprint(version), nil, c, nil, l)

		if err := config.SetSnapshot(nodeId, snap); err != nil {
			logs.Error("set snapshot err:", err)
		}

		reader := bufio.NewReader(os.Stdin)
		_, _ = reader.ReadString('\n')

	}
}

func RunManagementServer(ctx context.Context, server xds.Server, port uint) {
	var gRpcOptions []grpc.ServerOption
	gRpcOptions = append(gRpcOptions, grpc.MaxConcurrentStreams(gRpcMaxConcurrentStreams))
	gRpcServer := grpc.NewServer(gRpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logs.Error("failed to listen")
		return
	}

	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(gRpcServer, server)
	v2.RegisterEndpointDiscoveryServiceServer(gRpcServer, server)
	v2.RegisterClusterDiscoveryServiceServer(gRpcServer, server)
	v2.RegisterRouteDiscoveryServiceServer(gRpcServer, server)
	v2.RegisterListenerDiscoveryServiceServer(gRpcServer, server)

	logs.Debug("management server listening")
	go func() {
		if err = gRpcServer.Serve(lis); err != nil {
			logs.Error(err)
		}
	}()
	<-ctx.Done()

	gRpcServer.GracefulStop()
}

func RunAccessLogServer(ctx context.Context, als *aLogs.AccessLogService, port uint) {
	gRpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		logs.Error("failed to listen")
		return
	}

	alsV2.RegisterAccessLogServiceServer(gRpcServer, als)
	logs.Debug("access log server listening")

	go func() {
		if err = gRpcServer.Serve(lis); err != nil {
			logs.Error(err)
			return
		}
	}()
	<-ctx.Done()

	gRpcServer.GracefulStop()
}
