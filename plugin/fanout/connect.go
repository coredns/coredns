package fanout

import (
	"context"
	"github.com/coredns/coredns/request"
	"github.com/pkg/errors"
	"time"
)

func connect(ctx context.Context, client Client, req request.Request, result chan<- connectResult, maxFailCount int) {
	start := time.Now()
	var errs error
	for i := 0; i < maxFailCount+1; i++ {
		resp, err := client.Connect(req)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			result <- connectResult{
				client:   client,
				response: resp,
				start:    start,
			}
			return
		}
		if errs == nil {
			errs = err
		} else {
			errs = errors.Wrap(errs, err.Error())
		}
		if i < maxFailCount {
			if health := client.Health(); health != nil {
				if err := health.Check(); err != nil {
					HealthcheckBrokenCount.Add(1)
					errs = errors.Wrap(errs, err.Error())
					break
				}
			}
		}
	}
	result <- connectResult{
		client: client,
		err:    errs,
		start:  start,
	}
}
