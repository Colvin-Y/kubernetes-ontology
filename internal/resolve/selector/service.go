package selector

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

func ServiceSelectsPod(serviceID, podID model.CanonicalID) model.Edge {
	return model.NewEdge(serviceID, podID, model.EdgeKindSelectsPod)
}

func LabelsMatch(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}
