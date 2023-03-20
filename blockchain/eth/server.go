package eth

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/p2p/nat"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
)

// Server wraps the Ethereum p2p server, for use with the BDN
type Server struct {
	p2pServer           *p2p.Server
	cancel              context.CancelFunc
	dynamicPeerDisabled bool
}

// NewServer return an Ethereum p2p server, configured with BDN friendly defaults
func NewServer(parent context.Context, port int, externalIP net.IP, config *network.EthConfig, chain *Chain, bridge blockchain.Bridge, dataDir string, logger log.Logger, ws blockchain.WSManager, dynamicPeers int) (*Server, error) {
	var privateKey *ecdsa.PrivateKey

	if config.PrivateKey != nil {
		privateKey = config.PrivateKey
	} else {
		privateKeyPath := path.Join(dataDir, ".gatewaykey")
		privateKeyFromFile, generated, err := network.LoadOrGeneratePrivateKey(privateKeyPath)

		if err != nil {
			keyWriteErr, ok := err.(keyWriteError)
			if ok {
				logger.Warn("could not write private key", "err", keyWriteErr)
			} else {
				return nil, err
			}
		}

		if generated {
			logger.Warn("no private key found, generating new one", "path", privateKeyPath)
		}
		privateKey = privateKeyFromFile
	}

	ctx, cancel := context.WithCancel(parent)
	backend := NewHandler(ctx, config, chain, bridge, ws)

	var (
		discovery       = true
		dialRatio       = 1
		dynamicDisabled = false
	)

	// if no Dynamic peers we want to disable Dialing and Discovery
	if dynamicPeers == 0 {
		discovery = false
		dynamicDisabled = true
	}
	if dynamicPeers >= len(config.StaticEnodes()) {
		dialRatio = 2
	}

	server := p2p.Server{
		Config: p2p.Config{
			PrivateKey:       privateKey,
			MaxPeers:         dynamicPeers + len(config.StaticEnodes()),
			MaxPendingPeers:  0,
			DialRatio:        dialRatio,
			NoDiscovery:      !discovery,
			DiscoveryV5:      false,
			Name:             config.ProgramName,
			BootstrapNodesV5: nil,
			StaticNodes:      config.StaticEnodes(),
			TrustedNodes:     nil,
			NetRestrict:      nil,
			NodeDatabase:     "",
			Protocols:        MakeProtocols(ctx, backend),
			NAT:              nat.ExtIP(externalIP),
			Dialer:           nil,
			NoDial:           false,
			EnableMsgEvents:  false,
			Logger:           logger,
		},
		DiscV5: nil,
	}

	if discovery {
		server.Config.BootstrapNodes = config.BootstrapNodes
		server.Config.ListenAddr = fmt.Sprintf("0.0.0.0:%d", port)
	}

	s := &Server{
		p2pServer:           &server,
		cancel:              cancel,
		dynamicPeerDisabled: dynamicDisabled,
	}
	return s, nil
}

// NewServerWithEthLogger returns the p2p server preconfigured with the default Ethereum logger
func NewServerWithEthLogger(ctx context.Context, port int, externalIP net.IP, config *network.EthConfig, chain *Chain, bridge blockchain.Bridge, dataDir string, ws blockchain.WSManager, dynamicPeers int) (*Server, error) {
	l := log.New()
	l.SetHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(true)))

	return NewServer(ctx, port, externalIP, config, chain, bridge, dataDir, l, ws, dynamicPeers)
}

// Start starts eth server
func (s *Server) Start() error {
	if err := s.p2pServer.Start(); err != nil {
		return err
	}
	return nil
}

// Stop shutdowns the p2p server and any additional context relevant goroutines
func (s *Server) Stop() {
	s.cancel()
	s.p2pServer.Stop()
}

// AddEthLoggerFileHandler registers additional file handler by file path
func (s *Server) AddEthLoggerFileHandler(path string) error {
	fileHandler, err := log.FileHandler(path, log.TerminalFormat(false))
	if err != nil {
		return err
	}
	if s.dynamicPeerDisabled {
		s.p2pServer.Logger.SetHandler(log.LvlFilterHandler(log.LvlTrace, log.MultiHandler(fileHandler, s.p2pServer.Logger.GetHandler())))
	} else {
		s.p2pServer.Logger.SetHandler(log.LvlFilterHandler(log.LvlInfo, log.MultiHandler(fileHandler, s.p2pServer.Logger.GetHandler())))
	}

	return nil
}
