package injector

// Label keys
const (
	WebhookTargetKey = "injector.m213f.org/webhook"
)

// Annotation keys
const (
	// option
	RepoNameKey   = "injector.m213f.org/repository"
	BranchNameKey = "injector.m213f.org/branch"
	SourcePathKey = "injector.m213f.org/source"
	PruneFlagKey  = "injector.m213f.org/prune"

	// status
	SourceHashKey       = "injector.m213f.org/hash"
	SourceHashKeyPrefix = "injector.m213f.org/hash_"
)
