package injector

// Label keys
const (
	WebhookTargetKey = "injector.m213f.org/webhook"
)

// WebhookTargetValue is the label value of target secrets.
const WebhookTargetValue = "true"

// Annotation keys
const (
	LastUpdateKey = "injector.m213f.org/lastupdate"
	AutoPruneKey  = "injector.m213f.org/autoprune"
	RepositoryKey = "injector.m213f.org/repository"
	SourceKey     = "injector.m213f.org/source"
	BranchKey     = "injector.m213f.org/branch"

	HashKey       = "injector.m213f.org/hash"
	HashKeyPrefix = "injector.m213f.org/hash_"
)
