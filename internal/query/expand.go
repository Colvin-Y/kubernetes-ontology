package query

import (
	"context"
	"fmt"
	"time"

	"github.com/Colvin-Y/kubernetes-ontology/internal/api"
	"github.com/Colvin-Y/kubernetes-ontology/internal/model"
	"github.com/Colvin-Y/kubernetes-ontology/internal/ontology"
)

const (
	DefaultExpandDepth = 1
	MaxExpandDepth     = 4
	DefaultExpandLimit = 200
	MaxExpandLimit     = 2000
)

type ExpandOptions struct {
	EntityID     model.CanonicalID
	Depth        int
	Direction    ontology.Direction
	RelationKind string
	Limit        int
}

func ValidateExpandOptions(options ExpandOptions) error {
	if options.EntityID == "" {
		return fmt.Errorf("%w: entityGlobalId or id is required", ErrInvalidDiagnosticQuery)
	}
	if options.Depth < 0 {
		return fmt.Errorf("%w: depth must be >= 0", ErrInvalidDiagnosticQuery)
	}
	if options.Depth > MaxExpandDepth {
		return fmt.Errorf("%w: depth must be <= %d", ErrInvalidDiagnosticQuery, MaxExpandDepth)
	}
	if options.Limit < 0 {
		return fmt.Errorf("%w: limit must be >= 0", ErrInvalidDiagnosticQuery)
	}
	if options.Limit > MaxExpandLimit {
		return fmt.Errorf("%w: limit must be <= %d", ErrInvalidDiagnosticQuery, MaxExpandLimit)
	}
	switch options.Direction {
	case "", ontology.DirectionBoth, ontology.DirectionOut, ontology.DirectionIn:
	default:
		return fmt.Errorf("%w: direction must be in, out, or both", ErrInvalidDiagnosticQuery)
	}
	return nil
}

func ExpandSubgraph(ctx context.Context, backend ontology.Backend, options ExpandOptions) (api.GraphSubgraph, error) {
	if err := ValidateExpandOptions(options); err != nil {
		return api.GraphSubgraph{}, err
	}
	if options.Depth == 0 {
		options.Depth = DefaultExpandDepth
	}
	if options.Limit == 0 {
		options.Limit = DefaultExpandLimit
	}
	root, found, err := backend.GetEntity(ctx, options.EntityID)
	if err != nil {
		return api.GraphSubgraph{}, err
	}
	if !found {
		return api.GraphSubgraph{}, ErrDiagnosticEntryNotFound
	}

	nodes := map[model.CanonicalID]ontology.Entity{root.ID: root}
	edges := make(map[string]ontology.Relation)
	frontier := []model.CanonicalID{root.ID}
	expanded := make(map[model.CanonicalID]struct{})

	for depth := 0; depth < options.Depth && len(edges) < options.Limit; depth++ {
		nextSet := make(map[model.CanonicalID]struct{})
		for _, current := range frontier {
			if _, seen := expanded[current]; seen {
				continue
			}
			expanded[current] = struct{}{}
			remaining := options.Limit - len(edges)
			if remaining <= 0 {
				break
			}
			relations, err := backend.Neighbors(ctx, current, ontology.RelationQuery{
				Kind:      options.RelationKind,
				Direction: options.Direction,
				Limit:     remaining,
			})
			if err != nil {
				return api.GraphSubgraph{}, err
			}
			for _, relation := range relations {
				if len(edges) >= options.Limit {
					break
				}
				for _, id := range []model.CanonicalID{relation.From, relation.To} {
					if _, ok := nodes[id]; ok {
						continue
					}
					entity, found, err := backend.GetEntity(ctx, id)
					if err != nil {
						return api.GraphSubgraph{}, err
					}
					if found {
						nodes[id] = entity
					}
				}
				if _, fromOK := nodes[relation.From]; !fromOK {
					continue
				}
				if _, toOK := nodes[relation.To]; !toOK {
					continue
				}
				edges[relationKey(relation)] = relation
				for _, next := range nextIDs(current, relation, options.Direction) {
					if _, seen := expanded[next]; !seen {
						nextSet[next] = struct{}{}
					}
				}
			}
		}
		frontier = mapKeys(nextSet)
	}

	now := time.Now().UTC()
	return api.GraphSubgraph{
		Entry:       entityToDiagnosticNode(root),
		Nodes:       sortedDiagnosticNodes(nodes),
		Edges:       sortedDiagnosticEdges(edges),
		CollectedAt: &now,
		Explanation: []string{fmt.Sprintf("expanded %s by %d hop(s)", root.ID, options.Depth)},
	}, nil
}

func nextIDs(current model.CanonicalID, relation ontology.Relation, direction ontology.Direction) []model.CanonicalID {
	switch direction {
	case ontology.DirectionOut:
		if relation.From == current {
			return []model.CanonicalID{relation.To}
		}
	case ontology.DirectionIn:
		if relation.To == current {
			return []model.CanonicalID{relation.From}
		}
	default:
		if relation.From == current {
			return []model.CanonicalID{relation.To}
		}
		if relation.To == current {
			return []model.CanonicalID{relation.From}
		}
	}
	return nil
}
