package main

import (
	"context"
	"fmt"
	"github.com/bitmaelum/bitmaelum-suite/cmd/bm-server/handler"
	"github.com/bitmaelum/bitmaelum-suite/cmd/bm-server/handler/mgmt"
	"github.com/bitmaelum/bitmaelum-suite/cmd/bm-server/middleware"
	"github.com/bitmaelum/bitmaelum-suite/cmd/bm-server/processor"
	"github.com/bitmaelum/bitmaelum-suite/internal"
	"github.com/bitmaelum/bitmaelum-suite/internal/config"
	"github.com/bitmaelum/bitmaelum-suite/internal/container"
	"github.com/bitmaelum/bitmaelum-suite/internal/message"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type options struct {
	Config  string `short:"c" long:"config" description:"Configuration file" default:"./server-config.yml"`
	Version bool   `short:"v" long:"version" description:"Display version information"`
}

var opts options

func main() {
	rand.Seed(time.Now().UnixNano())

	internal.ParseOptions(&opts)
	if opts.Version {
		internal.WriteVersionInfo("BitMaelum Server", os.Stdout)
		fmt.Println()
		os.Exit(1)
	}

	config.LoadServerConfig(opts.Config)
	internal.SetLogging(config.Server.Logging.Level, config.Server.Logging.LogPath)

	logrus.Info("Starting " + internal.VersionString("bm-server"))

	_ = container.GetResolveService()

	// setup context so we can easily stop all components of the server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Wait for signals and cancel context
	setupSignals(cancel)

	// Start main loop processors
	logrus.Tracef("Starting main loop")
	go mainLoop(ctx)

	// Start HTTP server
	host := fmt.Sprintf("%s:%d", config.Server.Server.Host, config.Server.Server.Port)
	logrus.Tracef("Starting BitMaelum HTTP service on '%s'", host)
	go runHTTPService(ctx, cancel, host)

	// Clean up process queues
	logrus.Tracef("Waiting until context tells us it's done")
	<-ctx.Done()
	logrus.Tracef("Context is done. Exiting")
}

func setupSignals(cancel context.CancelFunc) {
	logrus.Tracef("Starting signal notifications")
	go func() {
		// Capture INT and TERM signals
		sigChannel := make(chan os.Signal, 1)
		signal.Notify(sigChannel, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP)

		for {
			s := <-sigChannel
			switch s {
			case syscall.SIGHUP:
				// Reload configuration and such
				logrus.Debug("SIGHUP received")
				internal.Reload()
			default:
				logrus.Infof("Signal %s received. Terminating.", s)
				cancel()
			}
		}
	}()
}

func setupRouter() *mux.Router {
	logger := &middleware.Logger{}
	tracer := &middleware.Tracer{}
	jwt := &middleware.JwtToken{}
	apikey := &middleware.APIKey{}
	prettyJSON := &middleware.PrettyJSON{}

	mainRouter := mux.NewRouter().StrictSlash(true)

	// Public things router
	publicRouter := mainRouter.PathPrefix("/").Subrouter()
	publicRouter.Use(logger.Middleware)
	publicRouter.Use(prettyJSON.Middleware)
	publicRouter.Use(tracer.Middleware)
	publicRouter.HandleFunc("/", handler.HomePage).Methods("GET")
	publicRouter.HandleFunc("/account", handler.CreateAccount).Methods("POST")
	publicRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/keys", handler.RetrieveKeys).Methods("GET")

	// Server to server message upload
	publicRouter.HandleFunc("/ticket", handler.GetLocalTicket).Methods("POST")
	publicRouter.HandleFunc("/incoming/header", handler.IncomingMessageHeader).Methods("POST")
	publicRouter.HandleFunc("/incoming/catalog", handler.IncomingMessageCatalog).Methods("POST")
	publicRouter.HandleFunc("/incoming/block/{message:[A-Za-z0-9-]+}", handler.IncomingMessageBlock).Methods("POST")
	publicRouter.HandleFunc("/incoming/attachment/{message:[A-Za-z0-9-]+}", handler.IncomingMessageAttachment).Methods("POST")
	publicRouter.HandleFunc("/incoming", handler.CompleteIncoming).Methods("POST")
	publicRouter.HandleFunc("/incoming", handler.DeleteIncoming).Methods("DELETE")

	// Routes that need to be authenticated
	authRouter := mainRouter.PathPrefix("/").Subrouter()
	authRouter.Use(logger.Middleware)
	authRouter.Use(prettyJSON.Middleware)
	authRouter.Use(tracer.Middleware)
	authRouter.Use(jwt.Middleware)
	// Authorized sending
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/ticket", handler.GetRemoteTicket).Methods("POST")
	// Message boxes
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/boxes", handler.RetrieveBoxes).Methods("GET")
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/boxes", handler.CreateBox).Methods("POST")
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/box/{box:[0-9+}", handler.DeleteBox).Methods("DELETE")
	// Message fetching
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/box/{box:[0-9]+}", handler.RetrieveMessagesFromBox).Methods("GET")
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/box/{box:[0-9]+}/message/{message:[A-Za-z0-9-]+}", handler.GetMessage).Methods("GET")
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/box/{box:[0-9]+}/message/{message:[A-Za-z0-9-]+}/block/{block:[A-Za-z0-9-]+}", handler.GetMessageBlock).Methods("GET")
	authRouter.HandleFunc("/account/{addr:[A-Za-z0-9]{64}}/box/{box:[0-9]+}/message/{message:[A-Za-z0-9-]+}/attachment/{attachment:[A-Za-z0-9-]+}", handler.GetMessageAttachment).Methods("GET")

	// Add management endpoints if enabled
	if config.Server.Management.Enabled {
		mgmtRouter := mainRouter.PathPrefix("/admin").Subrouter()
		mgmtRouter.Use(logger.Middleware)
		mgmtRouter.Use(prettyJSON.Middleware)
		mgmtRouter.Use(tracer.Middleware)
		mgmtRouter.Use(apikey.Middleware)

		mgmtRouter.HandleFunc("/flush", mgmt.FlushQueues).Methods("POST")
		mgmtRouter.HandleFunc("/invite", mgmt.NewInvite).Methods("POST")
		mgmtRouter.HandleFunc("/apikey", mgmt.NewAPIKey).Methods("POST")
	}

	return mainRouter
}

func runHTTPService(ctx context.Context, cancel context.CancelFunc, addr string) {
	router := setupRouter()

	// Wrap our router in Apache combined logging if needed
	var h http.Handler = router
	if config.Server.Logging.ApacheLogging == true {
		h = wrapWithApacheLogging(config.Server.Logging.ApacheLogPath, router)
	}

	// Setup HTTP server
	srv := &http.Server{Addr: addr, Handler: h}

	// Start serving TLS in go routine
	go func() {
		err := srv.ListenAndServeTLS(config.Server.Server.CertFile, config.Server.Server.KeyFile)
		if err != nil {
			logrus.Warn("HTTP server stopped: ", err)
			cancel()
		}
	}()

	logrus.Info("HTTP Server up and running")

	// Wait until the context is done
	for {
		select {
		case <-ctx.Done():
			logrus.Info("Shutting down the HTTP server...")
			_ = srv.Close()
			return
		}
	}
}

func mainLoop(ctx context.Context) {
	retryTicker := time.NewTicker(5 * time.Second)
	stuckTicker := time.NewTicker(60 * time.Second)
	processor.IncomingChannel = make(chan string)

	for {
		select {
		// Process incoming message
		case msgID := <-processor.IncomingChannel:
			logrus.Debugf("Message %s uploaded. Processing", msgID)
			err := message.MoveMessage(message.SectionIncoming, message.SectionProcessing, msgID)
			if err != nil {
				logrus.Tracef("issue while trying to move the message %s", err)
				continue
			}
			go processor.ProcessMessage(msgID)

		// Process tickers
		case <-retryTicker.C:
			go processor.ProcessRetryQueue(false)
		case <-stuckTicker.C:
			go processor.ProcessStuckIncomingMessages()
			go processor.ProcessStuckProcessingMessages()

		// Context is done (signal send)
		case <-ctx.Done():
			logrus.Info("Shutting down the processing queues...")
			return
		}
	}
}

// wrapWithApacheLogging wraps a router with a apache logger handler
func wrapWithApacheLogging(path string, router *mux.Router) http.Handler {
	w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
	if err != nil {
		w = os.Stderr
	}

	return handlers.CombinedLoggingHandler(w, router)
}
