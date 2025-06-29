package eth

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/nat"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/bsc"
	eth2 "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/protocols/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
)

// Server wraps the Ethereum p2p server, for use with the BDN
type Server struct {
	p2pServer           *p2p.Server
	cancel              context.CancelFunc
	dynamicPeerDisabled bool
}

// NewServer return an Ethereum p2p server, configured with BDN friendly defaults
func NewServer(parent context.Context, port int, externalIP net.IP, config *network.EthConfig, chain *core.Chain,
	bridge blockchain.Bridge, dataDir string, logger log.Logger, ws blockchain.WSManager, dynamicPeers, dialRatio int, recommendedPeers map[string]struct{},
) (*Server, error) {
	var privateKey *ecdsa.PrivateKey

	if config.PrivateKey != nil {
		privateKey = config.PrivateKey
	} else {
		privateKeyPath := path.Join(dataDir, ".gatewaykey")
		privateKeyFromFile, generated, err := network.LoadOrGeneratePrivateKey(privateKeyPath)
		if err != nil {
			var keyWriteErr network.KeyWriteError
			ok := errors.As(err, &keyWriteErr)
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
	backend := newHandler(ctx, config, chain, bridge, ws, recommendedPeers)

	var (
		discovery       = true
		dynamicDisabled = false
	)
	staticEnodes := config.StaticPeers.Enodes()

	// if no Dynamic peers we want to disable Dialing and Discovery
	if dynamicPeers == 0 {
		discovery = false
		dynamicDisabled = true
	}
	if dynamicPeers < len(staticEnodes) {
		dynamicPeersEnabled := dynamicPeers != 0
		if dynamicPeersEnabled && dialRatio != 1 {
			logger.Info("limit of peers to dial is less than number of enodes requested, dialRatio set to 1")
		}
		dialRatio = 1
	}

	server := p2p.Server{
		Config: p2p.Config{
			PrivateKey:       privateKey,
			MaxPeers:         dynamicPeers + len(staticEnodes),
			MaxPendingPeers:  0,
			DialRatio:        dialRatio,
			NoDiscovery:      !discovery,
			DiscoveryV5:      false,
			Name:             config.ProgramName,
			BootstrapNodesV5: nil,
			StaticNodes:      staticEnodes,
			TrustedNodes:     nil,
			NetRestrict:      nil,
			NodeDatabase:     "",
			Protocols:        makeProtocols(ctx, backend),
			NAT:              nat.ExtIP(externalIP),
			Dialer:           nil,
			NoDial:           false,
			EnableMsgEvents:  false,
			Logger:           logger,
		},
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

func makeProtocols(ctx context.Context, handler *handler) []p2p.Protocol {
	protos := eth2.MakeProtocols(ctx, (*ethHandler)(handler), handler.config.Network)

	if handler.config.Network == network.BSCMainnetChainID || handler.config.Network == network.BSCTestnetChainID {
		protos = append(protos, bsc.MakeProtocols(ctx, (*bscHandler)(handler))...)

	}

	return protos
}

// NewServerWithEthLogger returns the p2p server preconfigured with the default Ethereum logger
func NewServerWithEthLogger(ctx context.Context, port int, externalIP net.IP, config *network.EthConfig,
	chain *core.Chain, bridge blockchain.Bridge, dataDir string, ws blockchain.WSManager, dynamicPeers, dialRatio int, recommendedPeers map[string]struct{},
) (*Server, error) {
	l := log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stdout, log.LevelTrace, true))
	log.SetDefault(l)

	return NewServer(ctx, port, externalIP, config, chain, bridge, dataDir, l, ws, dynamicPeers, dialRatio, recommendedPeers)
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
	var err error
	var logOutputFile *os.File
	if logOutputFile, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err != nil {
		return err
	}
	output := io.MultiWriter(logOutputFile)
	handler := log.LogfmtHandler(output)

	glogger := log.NewGlogHandler(handler)

	if s.dynamicPeerDisabled {
		glogger.Verbosity(log.LevelTrace)
		err = glogger.Vmodule("p2p=5")
	} else {
		glogger.Verbosity(log.LevelInfo)
		err = glogger.Vmodule("p2p=3")
	}
	if err != nil {
		return fmt.Errorf("failed to set glog verbosity pattern: %w", err)
	}

	log.SetDefault(log.NewLogger(glogger))

	return nil
}
