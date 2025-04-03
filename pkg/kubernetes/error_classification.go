package kubernetes

import (
	"regexp"
)

// Regular expressions for error classification
var (
	rateLimitRegex   = regexp.MustCompile(`(?i)rate limit|rate limiter|too many requests|throttl|429|client rate limiter Wait.*context deadline exceeded`)
	leaderLostRegex  = regexp.MustCompile(`(?i)leader.*?lost|lost.*?leader|leader.*?change|lease.*?expire|stopped leading unexpectedly`)
	watchFailedRegex = regexp.MustCompile(`(?i)watch.*?fail|watch.*?close|watch.*?error|watch.*?stop|connection refused|connection reset`)
	timeoutRegex     = regexp.MustCompile(`(?i)timeout|timed out|deadline exceed`)
)

// ClassifyLeaderElectionError categorizes errors related to leader election
// by pattern matching against the error message
func ClassifyLeaderElectionError(err error) string {
	if err == nil {
		return "unknown"
	}
	
	errString := err.Error()
	
	// Rate limiting errors
	if rateLimitRegex.MatchString(errString) {
		return "rate_limit"
	}
	
	// Leadership loss errors
	if leaderLostRegex.MatchString(errString) {
		return "leadership_lost"
	}

	// Watch errors
	if watchFailedRegex.MatchString(errString) {
		return "watch_failure"
	}
	
	// Timeout errors
	if timeoutRegex.MatchString(errString) {
		return "timeout"
	}
	
	return "unknown"
}

// Legacy function to maintain backward compatibility
func classifyLeaderElectionError(err error) string {
	return ClassifyLeaderElectionError(err)
} 