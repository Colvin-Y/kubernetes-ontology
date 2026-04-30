import json
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
FIXTURE = ROOT / "samples" / "helm-upgrade-failure" / "diagnostic-graph.json"


class IncidentContextFixtureTest(unittest.TestCase):
    def test_helm_upgrade_failure_fixture_has_v1_metadata(self):
        data = json.loads(FIXTURE.read_text())
        self.assertEqual(data["schemaVersion"], "diagnostic.kubernetes-ontology.io/v1alpha1")
        self.assertEqual(data["recipe"], "helm-upgrade-runtime-failure")
        for field in ["warnings", "degradedSources", "budgets", "rankedEvidence", "conflicts", "freshness"]:
            self.assertIn(field, data)
        self.assertGreater(len(data["rankedEvidence"]), 0)
        self.assertGreater(len(data["lanes"]), 0)

    def test_helm_upgrade_failure_fixture_references_existing_graph_items(self):
        data = json.loads(FIXTURE.read_text())
        node_ids = {node["canonicalId"] for node in data["nodes"]}
        edge_keys = {f"{edge['from']}|{edge['kind']}|{edge['to']}" for edge in data["edges"]}

        self.assertIn(data["entry"]["canonicalId"], node_ids)
        for edge in data["edges"]:
            self.assertIn(edge["from"], node_ids)
            self.assertIn(edge["to"], node_ids)

        for item in data["rankedEvidence"]:
            if item.get("nodeId"):
                self.assertIn(item["nodeId"], node_ids)
            if item.get("edgeKey"):
                self.assertIn(item["edgeKey"], edge_keys)

        for conflict in data["conflicts"]:
            for node_id in conflict.get("nodeIds", []):
                self.assertIn(node_id, node_ids)
            for edge_key in conflict.get("edgeKeys", []):
                self.assertIn(edge_key, edge_keys)


if __name__ == "__main__":
    unittest.main()
