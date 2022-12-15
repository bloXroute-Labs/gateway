package eth

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"
	"path"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/nat"
)

// Server wraps the Ethereum p2p server, for use with the BDN
type Server struct {
	p2pServer *p2p.Server
	cancel    context.CancelFunc
}

// NewServer return an Ethereum p2p server, configured with BDN friendly defaults
func NewServer(parent context.Context, port int, externalIP net.IP, config *network.EthConfig, chain *Chain, bridge blockchain.Bridge, dataDir string, logger log.Logger, ws blockchain.WSManager, maxInboundConn int) (*Server, error) {
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
		maxDialedConn = len(config.StaticPeers)
		maxPeers      = maxDialedConn + maxInboundConn

		dialRatio int
		noDial    bool
	)

	if maxDialedConn > 0 {
		dialRatio = maxPeers / maxDialedConn
		noDial = false
	} else {
		noDial = true
	}

	server := p2p.Server{
		Config: p2p.Config{
			PrivateKey:       privateKey,
			MaxPeers:         maxPeers,
			MaxPendingPeers:  1,
			DialRatio:        dialRatio,
			NoDiscovery:      false,
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
			NoDial:           noDial,
			EnableMsgEvents:  false,
			Logger:           logger,
		},
		DiscV5: nil,
	}

	if maxInboundConn != 0 {
		server.Config.BootstrapNodes = config.BootstrapNodes
		server.Config.ListenAddr = fmt.Sprintf("0.0.0.0:%d", port)
	}

	s := &Server{
		p2pServer: &server,
		cancel:    cancel,
	}
	return s, nil
}

// NewServerWithEthLogger returns the p2p server preconfigured with the default Ethereum logger
func NewServerWithEthLogger(ctx context.Context, port int, externalIP net.IP, config *network.EthConfig, chain *Chain, bridge blockchain.Bridge, dataDir string, ws blockchain.WSManager, maxInboundConn int) (*Server, error) {
	l := log.New()
	l.SetHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(true)))

	return NewServer(ctx, port, externalIP, config, chain, bridge, dataDir, l, ws, maxInboundConn)
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
	s.p2pServer.Logger.SetHandler(log.LvlFilterHandler(log.LvlInfo, log.MultiHandler(fileHandler, s.p2pServer.Logger.GetHandler())))
	return nil
}
