// fanout_parallel.go — P5 coordination lifecycle: a BOUNDED parallel fan-out.
//
// NON-FROZEN (new file; not in the freeze manifest). This is the reference fan-out
// strategy registered into the plug-and-play `FanoutStrategy` hook (see providers.go
// header). It runs colony members CONCURRENTLY and, crucially, gives the coordinator
// a "stop / collect" lifecycle: a member that hangs is bounded by a budget and
// reported as a timeout while the rest still complete — the council is never held
// hostage by one stuck member. The kernel runtime instantiates a fresh module per
// Call, so concurrent invocation of distinct members is safe.
package loket

import (
	"encoding/json"
	"time"
)

// ParallelFanout invokes every target concurrently and collects replies, returning
// as soon as all finish OR the budget elapses (members still running are reported as
// a timeout — their goroutines unwind when the broadcast ctx is cancelled). Order of
// the result matches the input targets.
func ParallelFanout(budget time.Duration, targets []string, invoke func(target string) (json.RawMessage, error)) []FanoutBroadcastReply {
	out := make([]FanoutBroadcastReply, len(targets))
	got := make([]bool, len(targets))
	type res struct {
		i     int
		reply FanoutBroadcastReply
	}
	ch := make(chan res, len(targets))
	for i, t := range targets {
		go func(idx int, target string) {
			r, err := invoke(target)
			e := FanoutBroadcastReply{Target: target, Reply: r}
			if err != nil {
				e.Error = err.Error()
			}
			ch <- res{idx, e}
		}(i, t)
	}
	var deadline <-chan time.Time
	if budget > 0 {
		deadline = time.After(budget)
	}
	remaining := len(targets)
	for remaining > 0 {
		select {
		case rr := <-ch:
			out[rr.i] = rr.reply
			got[rr.i] = true
			remaining--
		case <-deadline:
			for i := range out {
				if !got[i] {
					out[i] = FanoutBroadcastReply{Target: targets[i], Error: "fan-out budget exceeded (member did not finish)"}
				}
			}
			return out
		}
	}
	return out
}
