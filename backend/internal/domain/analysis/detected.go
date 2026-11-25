package analysis

// DetectedEvidence is one detector finding attributed to the service whose
// incident it belongs to. Detectors produce it; the application groups it.
type DetectedEvidence struct {
	Service string
	Item    Evidence
}

// EntityID names an entity the way evidence and the topology refer to it, such as "service:checkout".
func EntityID(kind, name string) string { return kind + ":" + name }
