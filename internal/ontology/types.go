package ontology

import "github.com/Colvin-Y/kubernetes-ontology/internal/model"

type Direction string

const (
	DirectionBoth Direction = "both"
	DirectionOut  Direction = "out"
	DirectionIn   Direction = "in"
)

type EntityRef struct {
	ID        model.CanonicalID `json:"entityGlobalId,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
	Name      string            `json:"name,omitempty"`
}

type Entity struct {
	ID         model.CanonicalID `json:"entityGlobalId"`
	Kind       string            `json:"kind"`
	SourceKind string            `json:"sourceKind,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	Name       string            `json:"name,omitempty"`
	Attributes map[string]any    `json:"attributes,omitempty"`
}

type Relation struct {
	From       model.CanonicalID    `json:"from"`
	To         model.CanonicalID    `json:"to"`
	Kind       string               `json:"kind"`
	Provenance model.EdgeProvenance `json:"provenance"`
}

type EntityQuery struct {
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type RelationQuery struct {
	From      model.CanonicalID `json:"from,omitempty"`
	To        model.CanonicalID `json:"to,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Direction Direction         `json:"direction,omitempty"`
	Limit     int               `json:"limit,omitempty"`
}

type Stats struct {
	EntityCount   int `json:"entityCount"`
	RelationCount int `json:"relationCount"`
}

func EntityFromNode(node model.Node) Entity {
	return Entity{
		ID:         node.ID,
		Kind:       string(node.Kind),
		SourceKind: node.SourceKind,
		Namespace:  node.Namespace,
		Name:       node.Name,
		Attributes: node.Attributes,
	}
}

func NodeFromEntity(entity Entity) model.Node {
	return model.Node{
		ID:         entity.ID,
		Kind:       model.NodeKind(entity.Kind),
		SourceKind: entity.SourceKind,
		Namespace:  entity.Namespace,
		Name:       entity.Name,
		Attributes: entity.Attributes,
	}
}

func RelationFromEdge(edge model.Edge) Relation {
	return Relation{
		From:       edge.From,
		To:         edge.To,
		Kind:       string(edge.Kind),
		Provenance: edge.Provenance,
	}
}

func EdgeFromRelation(relation Relation) model.Edge {
	return model.Edge{
		From:       relation.From,
		To:         relation.To,
		Kind:       model.EdgeKind(relation.Kind),
		Provenance: relation.Provenance,
	}
}
