package service

import (
	"context"

	"github.com/Dynom/TySug/finder"
	"github.com/Dynom/TySug/keyboard"
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
	suggestions, score, exact := s.finder.FindTopRankingCtx(ctx, input)

	var suggestion string
	if len(suggestions) > 1 {
		suggestion, score = keyboard.New(keyboard.QwertyUS).FindNearest(input, suggestions)
		s.logger.WithFields(logrus.Fields{
			"input":                input,
			"1st_pass__suggestion": suggestions[0],
			"1st_pass_short_list":  suggestions,
			"2nd_pass_suggestion":  suggestion,
			"2nd_pass_score":       score,
		}).Debug("Had multiple suggestions, applied the keyboard distance to narrow down.")
	} else {
		suggestion = suggestions[0]
	}

	return suggestion, score, exact
}

func algJaroWinkler() finder.Algorithm {
	return func(a, b string) float64 {
		return smetrics.JaroWinkler(a, b, .7, 4)
	}
}
