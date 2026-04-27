# OWL Export

`kubernetes-ontology.owl` is a static RDF/XML OWL schema generated from the
current in-memory graph model:

```bash
make owl
```

The export covers:
- `model.NodeKind` as OWL classes
- `model.RelationSpec` / `model.EdgeKind` as OWL object properties, including
  domain/range, inverse relation, default provenance, and resolver hints
- core node attributes and edge provenance as datatype properties
- resolver/source/state annotations shared by the resolver edge constructors
  and OWL export

This is a schema-level export, not a live cluster fact dump. Runtime graph
instances can be materialized later by mapping each node `canonicalId` to an OWL
individual and each edge to either an object property assertion or a reified
`RelationAssertion` when provenance must be preserved.
