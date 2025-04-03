package kubernetes

import (
	"sync"
	"errors"
	"time"
)

// Regular expressions for error classification are now moved to error_classification.go

// SimulationMode represents different simulation behaviors for the fake client
type SimulationMode int

const (
	// NormalOperation - leader election works normally
	NormalOperation SimulationMode = iota
	// RateLimitError - simulate rate limit errors
	RateLimitError
	// LeadershipLossError - simulate leadership loss
	LeadershipLossError
	// WatchError - simulate watch errors
	WatchError
	// TimeoutError - simulate timeout errors
	TimeoutError
)

// FakeClientForTest is a very simple test helper that overrides parts of the client
// to simulate leader election scenarios without mocking internal components
type FakeClientForTest struct {
	currentLeader    string
	Mode             SimulationMode
	mu               sync.Mutex
	failureCount     int
	maxFailures      int
	errorCh          chan error
	clientToInject   *Client
}

// NewFakeClientForTest creates a new fake client helper
func NewFakeClientForTest() *FakeClientForTest {
	return &FakeClientForTest{
		Mode:          NormalOperation,
		maxFailures:   1,
	}
}

// SetSimulationMode sets the simulation mode and resets failure count
func (f *FakeClientForTest) SetSimulationMode(mode SimulationMode) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Mode = mode
	f.failureCount = 0
}

// SetMaxFailures sets how many errors to generate before returning to normal operation
func (f *FakeClientForTest) SetMaxFailures(count int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maxFailures = count
}

// SetLeader sets the current leader identity
func (f *FakeClientForTest) SetLeader(identity string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.currentLeader = identity
}

// GetLeader gets the current leader identity
func (f *FakeClientForTest) GetLeader() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.currentLeader
}

// InjectIntoClient applies the test helper to a real client to simulate behaviors
func (f *FakeClientForTest) InjectIntoClient(client *Client) {
	f.mu.Lock()
	f.clientToInject = client
	f.mu.Unlock()
	
	// Start a goroutine to simulate leader election errors
	go f.simulateErrors()
}

// simulateErrors periodically checks if we need to simulate errors
func (f *FakeClientForTest) simulateErrors() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for range ticker.C {
		f.mu.Lock()
		
		// Skip if client isn't set yet or we're not in an error mode
		if f.clientToInject == nil || f.Mode == NormalOperation || f.failureCount >= f.maxFailures {
			f.mu.Unlock()
			continue
		}
		
		// Generate an error based on the current mode
		var err error
		switch f.Mode {
		case RateLimitError:
			err = errors.New("client rate limiter Wait returned an error: context deadline exceeded")
			// Make client not the leader to simulate rate limit effect
			f.clientToInject.leadershipMutex.Lock()
			f.clientToInject.isLeader = false
			f.clientToInject.leadershipMutex.Unlock()
		case LeadershipLossError:
			err = errors.New("stopped leading unexpectedly")
			// Make client not the leader to simulate leadership loss
			f.clientToInject.leadershipMutex.Lock()
			f.clientToInject.isLeader = false
			f.clientToInject.leadershipMutex.Unlock()
		case WatchError:
			err = errors.New("Error watching nodes: connection refused")
			// Make client not the leader to simulate watch error effect
			f.clientToInject.leadershipMutex.Lock()
			f.clientToInject.isLeader = false
			f.clientToInject.leadershipMutex.Unlock()
		case TimeoutError:
			err = errors.New("context deadline exceeded")
			// Make client not the leader to simulate timeout effect
			f.clientToInject.leadershipMutex.Lock()
			f.clientToInject.isLeader = false
			f.clientToInject.leadershipMutex.Unlock()
		}
		
		// Increment the failure count
		f.failureCount++
		
		// Get references to client before releasing mutex
		client := f.clientToInject
		f.mu.Unlock()
		
		// Process the error as if it came from real leader election
		// This directly exercises the actual error handling code
		client.incrementLeaderElectionError(ClassifyLeaderElectionError(err))
		client.incrementLeaderElectionRestart()
		
		// Let error handling code make the client leader again after a delay
		time.Sleep(3 * time.Second)
		client.leadershipMutex.Lock()
		client.isLeader = true
		client.leadershipMutex.Unlock()
		f.SetLeader(getHostname())
	}
}

// Utility function to classify leader election errors is now defined in error_classification.go 