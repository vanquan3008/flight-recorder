package analysis

// DetectedEvidence is one detector finding attributed to the service whose
// incident it belongs to. Detectors produce it; the application groups it.
type DetectedEvidence struct {
	Service string
	Item    Evidence
}
