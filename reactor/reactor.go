package reactor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	openapi "github.com/aep/apogy/api/go"
)

type Runtime interface {
	Ready(model *openapi.Document, arg interface{}) (interface{}, error)
	Validate(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error)
	Reconcile(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) error
	Stop()
}

type reactorReadyArgs struct {
	reactorName string
	args        interface{}
}

type Reactor struct {
	lock            sync.RWMutex
	running         map[string]Runtime
	models2reactors map[string][]reactorReadyArgs

	tlsConfig *tls.Config
	tlsClient *http.Client
}

func NewReactor(caCertPath, serverCertPath, serverKeyPath string) *Reactor {
	ro := &Reactor{
		running:         make(map[string]Runtime),
		models2reactors: make(map[string][]reactorReadyArgs),
	}

	// Load TLS configuration if paths are provided
	if caCertPath != "" && serverCertPath != "" && serverKeyPath != "" {
		// Load the CA cert
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			slog.Error("failed to read CA certificate", "error", err)
		} else {
			// Create a cert pool and add the CA cert
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				slog.Error("failed to append CA certificate to pool")
			} else {
				// Load client certificate
				cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
				if err != nil {
					slog.Error("failed to load client certificate/key", "error", err)
				} else {
					// Create a TLS config with the certificate and key
					ro.tlsConfig = &tls.Config{
						Certificates: []tls.Certificate{cert},
						RootCAs:      caCertPool,
					}

					// Create a reusable HTTP client with TLS configuration
					ro.tlsClient = &http.Client{
						Transport: &http.Transport{
							TLSClientConfig: ro.tlsConfig,
						},
					}
					slog.Info("TLS configuration loaded successfully")
				}
			}
		}
	}

	ro.startBuiltins()
	return ro
}

func (ro *Reactor) reactorReadyArgsFromModel(doc *openapi.Document) ([]reactorReadyArgs, error) {

	val, _ := doc.Val.(map[string]interface{})
	if val == nil {
		return nil, nil
	}

	ro.lock.RLock()
	defer ro.lock.RUnlock()

	var ret []reactorReadyArgs

	ii, _ := val["reactors"].([]interface{})

	if val["cue"] != nil {
		ii = append([]interface{}{"cue"}, ii...)
	}

	if val["schema"] != nil {
		ii = append([]interface{}{"schema"}, ii...)
	}

	for _, i := range ii {

		switch i := i.(type) {
		case string:
			reactorName := i
			if ro.running[reactorName] == nil {
				return nil, fmt.Errorf("reactor does not exist: %s", reactorName)
			}
			args, err := ro.running[reactorName].Ready(doc, nil)
			if err != nil {
				return nil, fmt.Errorf("invalid use of reactor %s: %w", reactorName, err)
			}
			ret = append(ret, reactorReadyArgs{reactorName: reactorName, args: args})
		case map[string]interface{}:
			for k, v := range i {
				reactorName := k
				if ro.running[reactorName] == nil {
					return nil, fmt.Errorf("reactor does not exist: %s", reactorName)
				}
				args, err := ro.running[reactorName].Ready(doc, v)
				if err != nil {
					return nil, fmt.Errorf("invalid use of reactor %s: %w", reactorName, err)
				}
				ret = append(ret, reactorReadyArgs{reactorName: reactorName, args: args})

				break
			}
		}
	}

	return ret, nil

}

func (ro *Reactor) Validate(ctx context.Context, old *openapi.Document, nuw *openapi.Document) (*openapi.Document, error) {

	if nuw != nil && nuw.Model == "Model" {
		_, err := ro.reactorReadyArgsFromModel(nuw)
		if err != nil {
			return nil, err
		}
	}

	if nuw != nil && nuw.Model == "Reactor" {
		err := ro.start(ctx, nuw)
		return nuw, err
	} else if old != nil && old.Model == "Reactor" {
		err := ro.start(ctx, old)
		return old, err
	}

	var modelName string
	if nuw == nil {
		modelName = old.Model
	} else {
		modelName = nuw.Model
	}

	ro.lock.RLock()
	pp := ro.models2reactors[modelName]
	ro.lock.RUnlock()

	for _, runArgs := range pp {

		ro.lock.RLock()
		rt := ro.running[runArgs.reactorName]
		ro.lock.RUnlock()

		if rt != nil {
			var err error
			nuw, err = rt.Validate(ctx, ro, old, nuw, runArgs.args)
			if err != nil {
				return nuw, fmt.Errorf("reactor %s rejected change: %w", runArgs.reactorName, err)
			}
		}
	}

	return nuw, nil
}

// FIXME: reconcilers actually need to be durable i.e. call them forever until it succeeds but i need distributed locking first

func (ro *Reactor) Reconcile(ctx context.Context, old *openapi.Document, nuw *openapi.Document) error {

	var modelName string
	if nuw == nil {
		modelName = old.Model
	} else {
		modelName = nuw.Model
	}

	if nuw == nil && old != nil && old.Model == "Model" {

		ro.lock.Lock()
		delete(ro.models2reactors, old.Id)
		ro.lock.Unlock()

	} else if nuw != nil && nuw.Model == "Model" {
		aa, err := ro.reactorReadyArgsFromModel(nuw)
		if err != nil {
			return err
		}
		ro.lock.Lock()
		ro.models2reactors[nuw.Id] = aa
		ro.lock.Unlock()
	}

	ro.lock.RLock()
	aa := ro.models2reactors[modelName]
	ro.lock.RUnlock()

	for _, a := range aa {

		ro.lock.RLock()
		rt := ro.running[a.reactorName]
		ro.lock.RUnlock()

		if rt != nil {

			var delay = 10 * time.Millisecond
			for i := 0; ; i++ {

				err := rt.Reconcile(ctx, ro, old, nuw, a.args)

				if err == nil {
					break
				}
				slog.Warn("reconciler", "name", a.reactorName, "err", err)
				if i > 10 {
					return err
				}
				time.Sleep(delay)
				delay = 2 * delay
			}
		}
	}

	return nil
}

func (ro *Reactor) stop(ctx context.Context, rd *openapi.Document) error {

	ro.lock.Lock()
	ded := ro.running[rd.Id]
	delete(ro.running, rd.Id)
	ro.lock.Unlock()

	if ded != nil {
		slog.Info("stopping reactor", "id", rd.Id)
		ded.Stop()
	}

	return nil
}

func (ro *Reactor) start(ctx context.Context, rd *openapi.Document) error {

	val, _ := rd.Val.(map[string]interface{})
	if val == nil {
		return fmt.Errorf("invalid reactor: val.runtime is required")
	}

	runtimeName, ok := val["runtime"].(string)
	if !ok || runtimeName == "" {
		return fmt.Errorf("invalid reactor: val.runtime must be string")
	}

	var started Runtime
	if runtimeName == "http" {
		var err error
		started, err = StartHttpReactor(rd)
		if err != nil {
			return err
		}
		slog.Info("started http reactor", "id", rd.Id)
	} else {
		return fmt.Errorf("invalid reactor: runtime %s not supported", runtimeName)
	}

	ro.lock.Lock()
	old := ro.running[rd.Id]
	ro.running[rd.Id] = started
	ro.lock.Unlock()

	if old != nil {
		old.Stop()
	}

	return nil
}

func (ro *Reactor) Status(ctx context.Context, rd *openapi.Document) {

	if rd.Model != "Reactor" {
		return
	}

	ro.lock.RLock()
	has := ro.running[rd.Id]
	ro.lock.RUnlock()

	s := map[string]interface{}{
		"reactor": map[string]interface{}{
			"running": has != nil,
		},
	}
	rd.Status = &s
}
