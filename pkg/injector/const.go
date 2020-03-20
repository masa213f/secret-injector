package injector

// KeyPrefix is the prefix of label and annotation keys.
const KeyPrefix = "injector.m213f.org/"

// TargetLabelKey is the label key of target secrets.
const TargetLabelKey = KeyPrefix + "injection"

// TargetLabelValue is the label value of target secrets.
const TargetLabelValue = "true"

// AnnotationKey is a annotation key.
const AnnotationKey = KeyPrefix + "update-timestamp"
