package server

import (
	"encoding/json"
	"net/http"

	"time"

	"io"
	"io/ioutil"

	"errors"

	"github.com/sirupsen/logrus"
	"gopkg.in/tomb.v2"
)

const maxBodySize = 1 << 20 // 1Mb

// Errors
var (
	ErrMissingBody        = errors.New("missing body")
	ErrInvalidRequest     = errors.New("invalid request")
	ErrInvalidRequestBody = errors.New("invalid request body")
)

type contextKey int

// Context value parameters
const (
	CtxRequestID contextKey = iota
)

type tySugRequest struct {
	Input string `json:"input"`
}

type tySugResponse struct {
	Result string  `json:"result"`
	Score  float64 `json:"score"`
}

// Validator is a type to validate a client request, returning a nil errors means all went well.
type Validator func(TSRequest tySugRequest) error

// TySugServer the HTTP server
type TySugServer struct {
	server     *http.Server
	handlers   []func(h http.Handler) http.Handler
	validators []Validator

	Logger *logrus.Logger
}

// ListenOnAndServe allows to set the host:port URL late. It calls ListenAndServe()
func (tss *TySugServer) ListenOnAndServe(addr string) error {
	tss.server.Addr = addr

	return tss.server.ListenAndServe()
}

// NewHTTP constructs a new TySugServer
func NewHTTP(sr ServiceRegistry, mux *http.ServeMux, options ...Option) TySugServer {
	tySug := TySugServer{
		Logger: logrus.StandardLogger(),
	}

	for _, opt := range options {
		opt(&tySug)
	}

	var handler http.Handler = defaultHeaderHandler(createRequestIDHandler(mux))
	for _, h := range tySug.handlers {
		handler = h(handler)
	}

	mux.HandleFunc("/", http.NotFound)
	mux.HandleFunc("/list/", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[6:]
		if name == "" {
			tySug.Logger.Info("no list name defined")
			w.WriteHeader(400)
			return
		}

		if !sr.HasServiceForList(name) {
			tySug.Logger.Infof("list '%s' not defined", name)
			w.WriteHeader(404)
			return
		}

		svc := sr.GetServiceForList(name)
		hf := createRequestHandler(
			tySug.Logger,
			svc,
			tySug.validators,
		)

		hf(w, r)
	})

	tySug.server = &http.Server{
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 19, // 512 kb
		Handler:           handler,
	}

	return tySug
}

func createRequestHandler(logger *logrus.Logger, svc Service, validators []Validator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, ctx := tomb.WithContext(r.Context())

		req, reqErr := getRequestFromHTTPRequest(r)
		if reqErr != nil {
			w.WriteHeader(400)
			_, writeErr := w.Write([]byte(reqErr.Error()))
			if writeErr != nil {
				logger.Errorf("Error while writing 400 error: %s (original error: %q)", writeErr, reqErr)
			}
			return
		}

		for _, v := range validators {
			if vErr := v(req); vErr != nil {
				logger.WithFields(logrus.Fields{
					"error": vErr,
				}).Error("Request validation failed")

				w.WriteHeader(400)
				_, writeErr := w.Write([]byte("Validation failed."))
				if writeErr != nil {
					logger.Errorf("Error while writing 400 error: %s", writeErr)
				}
				return
			}
		}

		var res tySugResponse

		start := time.Now()
		res.Result, res.Score = svc.Find(ctx, req.Input)

		logger.WithFields(logrus.Fields{
			"input":       req.Input,
			"suggestion":  res.Result,
			"score":       res.Score,
			"duration_µs": time.Since(start) / time.Microsecond,
			"request_id":  ctx.Value(CtxRequestID),
		}).Debug("Completed new ranking request")

		if !t.Alive() {
			logger.WithFields(logrus.Fields{
				"request_id": ctx.Value(CtxRequestID),
			}).Info("Request got cancelled")
		}

		response, err := json.Marshal(res)
		if err != nil {
			w.WriteHeader(500)
			_, writeErr := w.Write([]byte("unable to marshal result, b00m"))
			logger.Errorf("Error while writing 500 error: %s (original marshaling error: %q)", writeErr, err)
			return
		}

		_, err = w.Write(response)
		if err != nil {
			logger.Errorf("Error while writing response: %s", err)
		}
	}
}

func getRequestFromHTTPRequest(r *http.Request) (tySugRequest, error) {
	var req tySugRequest

	b, err := ioutil.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		if err == io.EOF {
			return req, ErrMissingBody
		}
		return req, ErrInvalidRequest
	}

	err = json.Unmarshal(b, &req)
	if err != nil {
		return req, ErrInvalidRequestBody
	}

	return req, nil
}
