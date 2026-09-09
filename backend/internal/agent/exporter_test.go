package agent

import "testing"

func TestNewExporterAddsMachineResourceAttributes(t *testing.T) {
	exporter, newError := NewExporter("localhost:4317", "", "file-agent", "host-a", "/var/log/auth.log", false, "")
	if newError != nil {
		t.Fatal(newError)
	}
	defer exporter.Close()
	attributes := make(map[string]string, len(exporter.resourceAttributes))
	for _, attribute := range exporter.resourceAttributes {
		attributes[attribute.Key] = attribute.Value.GetStringValue()
	}
	if attributes["host.name"] != "host-a" || attributes["log.file.path"] != "/var/log/auth.log" || attributes["log.source.type"] != "file" {
		t.Fatalf("machine metadata = %#v", attributes)
	}
}
