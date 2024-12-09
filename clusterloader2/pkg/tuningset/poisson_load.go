/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tuningset

import (
	"math"
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"github.com/deekshith-elear/perf-tests/clusterloader2/api"
)

type poissonLoad struct {
	params *api.PoissonLoad
}

func newPoissonLoad(params *api.PoissonLoad) TuningSet {
	return &poissonLoad{
		params: params,
	}
}

func (rl *poissonLoad) Execute(actions []func()) {
	var wg wait.Group
	actionNextLaunch := time.Now()
	for i := range actions {
		time.Sleep(time.Until(actionNextLaunch))
		wg.Start(actions[i])
		actionNextLaunch = actionNextLaunch.Add(interArrivalTime(rl.params.ExpectedActionsPerSecond))
	}
	wg.Wait()
}

// Simulating inter-arrival times in a Poisson process
func interArrivalTime(MeanRate float64) time.Duration {
	p := rand.Float64()
	actionInterArrrivalTime := time.Duration(int(float64(time.Second) * (-math.Log(1-p) / MeanRate)))
	return actionInterArrrivalTime
}
