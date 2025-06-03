package testutil

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

var (
	portManageInstance *portManager
	once               sync.Once
)

type portManager struct {
	usedPorts map[int]bool
	mu        sync.RWMutex
	minPort   int
	maxPort   int
	rng       *rand.Rand
}

func getPortManager() *portManager {
	once.Do(func() {
		portManageInstance = &portManager{
			usedPorts: make(map[int]bool),
			minPort:   15000,
			maxPort:   25000,
			rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		}
	})
	return portManageInstance
}

func (pm *portManager) reservePort() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for attempts := 0; attempts < 50; attempts++ {
		port := pm.minPort + pm.rng.Intn(pm.maxPort-pm.minPort+1)
		if !pm.usedPorts[port] {
			pm.usedPorts[port] = true
			return port, nil
		}
	}

	return 0, fmt.Errorf("failed to find available port after 50 attempts")
}

func (pm *portManager) releasePort(port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.usedPorts, port)
}

func (pm *portManager) reset() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.usedPorts = make(map[int]bool)
}
