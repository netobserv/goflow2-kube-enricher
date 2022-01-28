package pipe

import "github.com/vmware/go-ipfix/pkg/entities"

func Transform(in <-chan *entities.Message) <-chan map[string]interface{} {
	out := make(chan map[string]interface{})
	go func() {
		for msg := range in {
			time := msg.GetExportTime()
			for _, record := range msg.GetSet().GetRecords() {
				ipfixFlow := record.GetElementMap()
				out <- map[string]interface{}{
					"Timestamp":     time,
					"SrcAddr":       ipfixFlow["sourceIPv4Address"],
					"TimeFlowStart": ipfixFlow["flowStartSeconds"],
					"TimeFlowEnd":   ipfixFlow["flowEndSeconds"],
				}
			}
		}
	}()
	return out
}
