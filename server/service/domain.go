package service

import (
	"context"

	"github.com/Dynom/TySug/finder"
	"github.com/sirupsen/logrus"
	"github.com/xrash/smetrics"
)

// NewDomain creates a new service
func NewDomain(references []string, logger *logrus.Logger, options ...finder.Option) (Service, error) {
	defaults := []finder.Option{
		finder.WithAlgorithm(algJaroWinkler()),
		finder.WithLengthTolerance(0.2),
	}

	scorer, err := finder.New(references, append(defaults, options...)...)
	if err != nil {
		return Service{}, err
	}

	return Service{
		scorer,
		logger,
	}, nil
}

// Service is the service type
type Service struct {
	finder *finder.Finder
	logger *logrus.Logger
}

// Find returns the nearest reference
func (s Service) Find(ctx context.Context, input string) (string, float64, bool) {
	suggestion, score, exact := s.finder.FindCtx(ctx, input)
	return suggestion, score, exact
}

func algJaroWinkler() finder.Algorithm {
	return func(a, b string) float64 {
		return smetrics.JaroWinkler(a, b, .7, 4)
	}
}
