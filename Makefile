BINARY := bin/kubernetes-ontology
DAEMON_BINARY := bin/kubernetes-ontologyd
VIEWER_BINARY := bin/kubernetes-ontology-viewer
OWL_FILE ?= docs/ontology/kubernetes-ontology.owl

DEFAULT_CONFIG := local/kubernetes-ontology.yaml
CONFIG ?= $(if $(wildcard $(DEFAULT_CONFIG)),$(DEFAULT_CONFIG),)
KUBECONFIG ?=
CLUSTER ?= default-cluster
NAMESPACE ?=
NAMESPACES ?=
NAME ?=
CONTEXT_NAMESPACES ?= $(if $(NAMESPACES),$(NAMESPACES),$(NAMESPACE))
WORKLOAD_RESOURCES ?=
CONTROLLER_RULES ?=
MAX_DEPTH ?= 2
STORAGE_MAX_DEPTH ?= 5
MAX_NODES ?=
MAX_EDGES ?=
BOOTSTRAP_TIMEOUT ?= 2m
OBSERVE_DURATION ?= 40s
POLL_INTERVAL ?= 2s
VIEWER_HOST ?= 127.0.0.1
VIEWER_PORT ?= 8765
SERVER_ADDR ?= 127.0.0.1:18080
SERVER_URL ?= http://127.0.0.1:18080
VIEWER_URL ?= http://$(VIEWER_HOST):$(VIEWER_PORT)
ENTITY_ID ?=
GRAPH_FILE ?=
ENTITY_KIND ?=
RELATION_KIND ?=
FROM_ID ?=
TO_ID ?=
DIRECTION ?= both
LIMIT ?= 50
EXPAND_DEPTH ?= 1

OVERRIDE_ORIGINS := command line environment override
make_arg = $(if $(filter $(OVERRIDE_ORIGINS),$(origin $(1))),--$(2) "$($(1))")
make_context_namespaces_arg = $(if $(filter $(OVERRIDE_ORIGINS),$(origin CONTEXT_NAMESPACES) $(origin NAMESPACES)),--context-namespaces "$(CONTEXT_NAMESPACES)")
make_diagnostic_budget_args = $(call make_arg,MAX_NODES,max-nodes) $(call make_arg,MAX_EDGES,max-edges)
CLI_CONFIG_OVERRIDES = $(call make_arg,KUBECONFIG,kubeconfig) $(call make_arg,CLUSTER,cluster) $(make_context_namespaces_arg) $(call make_arg,WORKLOAD_RESOURCES,workload-resources) $(call make_arg,CONTROLLER_RULES,controller-rules) $(call make_arg,BOOTSTRAP_TIMEOUT,bootstrap-timeout)
DAEMON_CONFIG_OVERRIDES = $(CLI_CONFIG_OVERRIDES) $(call make_arg,SERVER_ADDR,addr) $(call make_arg,POLL_INTERVAL,poll-interval)
CLI_CONFIG_ARGS = $(if $(CONFIG),--config "$(CONFIG)" $(CLI_CONFIG_OVERRIDES),--kubeconfig "$(KUBECONFIG)" --cluster "$(CLUSTER)" --context-namespaces "$(CONTEXT_NAMESPACES)" --workload-resources "$(WORKLOAD_RESOURCES)" --controller-rules "$(CONTROLLER_RULES)" --bootstrap-timeout "$(BOOTSTRAP_TIMEOUT)")
DAEMON_CONFIG_ARGS = $(if $(CONFIG),--config "$(CONFIG)" $(DAEMON_CONFIG_OVERRIDES),--kubeconfig "$(KUBECONFIG)" --cluster "$(CLUSTER)" --context-namespaces "$(CONTEXT_NAMESPACES)" --workload-resources "$(WORKLOAD_RESOURCES)" --controller-rules "$(CONTROLLER_RULES)" --addr "$(SERVER_ADDR)" --bootstrap-timeout "$(BOOTSTRAP_TIMEOUT)" --poll-interval "$(POLL_INTERVAL)")

.PHONY: build build-daemon build-viewer docker-build owl test verify ci ci-go ci-helm ci-binaries ci-client ci-visualize run serve status status-server list-entities-server get-entity-server list-relations-server neighbors-server expand-node-server collapse-node-graph observe-status diagnose-pod diagnose-workload diagnose-helm-release diagnose-pod-server diagnose-workload-server visualize visualize-go visualize-check live-check verify-live require-kubeconfig require-entry

build:
	mkdir -p bin
	go build -o $(BINARY) ./cmd/kubernetes-ontology

build-daemon:
	mkdir -p bin
	go build -o $(DAEMON_BINARY) ./cmd/kubernetes-ontologyd

build-viewer:
	mkdir -p bin
	go build -o $(VIEWER_BINARY) ./cmd/kubernetes-ontology-viewer

docker-build:
	docker build -t kubernetes-ontology:dev .

owl:
	go run ./cmd/kubernetes-ontology-owl --output "$(OWL_FILE)"

test:
	go test -p 1 ./...

verify: test visualize-check

ci: ci-go ci-helm ci-binaries ci-client ci-visualize

ci-go: test

ci-helm:
	bash scripts/ci/verify_helm.sh

ci-binaries:
	bash scripts/ci/verify_binaries.sh

ci-client: build
	bash scripts/ci/verify_client.sh

ci-visualize: build-viewer visualize-check
	bash scripts/ci/verify_viewer.sh

run: build
	$(BINARY) $(ARGS)

serve: build-daemon require-kubeconfig
	$(DAEMON_BINARY) $(DAEMON_CONFIG_ARGS)

status: build require-kubeconfig
	$(BINARY) $(CLI_CONFIG_ARGS) --status-only

status-server: build
	$(BINARY) --server "$(SERVER_URL)" --status-only

list-entities-server: build
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --list-entities \
	  --entity-kind "$(ENTITY_KIND)" \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)" \
	  --limit "$(LIMIT)"

get-entity-server: build
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --get-entity \
	  --entity-id "$(ENTITY_ID)" \
	  --entity-kind "$(ENTITY_KIND)" \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)"

list-relations-server: build
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --list-relations \
	  --from "$(FROM_ID)" \
	  --to "$(TO_ID)" \
	  --relation-kind "$(RELATION_KIND)" \
	  --limit "$(LIMIT)"

neighbors-server: build
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --neighbors \
	  --entity-id "$(ENTITY_ID)" \
	  --relation-kind "$(RELATION_KIND)" \
	  --direction "$(DIRECTION)" \
	  --limit "$(LIMIT)"

expand-node-server: build
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --expand-node \
	  --entity-id "$(ENTITY_ID)" \
	  --expand-depth "$(EXPAND_DEPTH)" \
	  --relation-kind "$(RELATION_KIND)" \
	  --direction "$(DIRECTION)" \
	  --limit "$(LIMIT)"

collapse-node-graph: build
	$(BINARY) \
	  --collapse-node \
	  --graph-file "$(GRAPH_FILE)" \
	  --entity-id "$(ENTITY_ID)"

observe-status: build require-kubeconfig
	$(BINARY) \
	  $(CLI_CONFIG_ARGS) \
	  --status-only \
	  --observe-duration "$(OBSERVE_DURATION)" \
	  --poll-interval "$(POLL_INTERVAL)"

diagnose-pod: build require-kubeconfig require-entry
	$(BINARY) \
	  $(CLI_CONFIG_ARGS) \
	  --entry-kind Pod \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)" \
	  --max-depth "$(MAX_DEPTH)" \
	  --storage-max-depth "$(STORAGE_MAX_DEPTH)" $(make_diagnostic_budget_args)

diagnose-workload: build require-kubeconfig require-entry
	$(BINARY) \
	  $(CLI_CONFIG_ARGS) \
	  --entry-kind Workload \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)" \
	  --max-depth "$(MAX_DEPTH)" \
	  --storage-max-depth "$(STORAGE_MAX_DEPTH)" $(make_diagnostic_budget_args)

diagnose-helm-release: build require-kubeconfig require-entry
	$(BINARY) \
	  $(CLI_CONFIG_ARGS) \
	  --diagnose-helm-release \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)" \
	  --max-depth "$(MAX_DEPTH)" \
	  --storage-max-depth "$(STORAGE_MAX_DEPTH)" $(make_diagnostic_budget_args)

diagnose-pod-server: build require-entry
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --entry-kind Pod \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)" \
	  --max-depth "$(MAX_DEPTH)" \
	  --storage-max-depth "$(STORAGE_MAX_DEPTH)" $(make_diagnostic_budget_args)

diagnose-workload-server: build require-entry
	$(BINARY) \
	  --server "$(SERVER_URL)" \
	  --entry-kind Workload \
	  --namespace "$(NAMESPACE)" \
	  --name "$(NAME)" \
	  --max-depth "$(MAX_DEPTH)" \
	  --storage-max-depth "$(STORAGE_MAX_DEPTH)" $(make_diagnostic_budget_args)

visualize:
	@echo "Starting ontology viewer on http://$(VIEWER_HOST):$(VIEWER_PORT)"
	@echo "Default ontology server: $(SERVER_URL)"
	@echo "Press Ctrl+C to stop the viewer."
	ONTOLOGY_SERVER="$(SERVER_URL)" python3 tools/visualize/server.py --host "$(VIEWER_HOST)" --port "$(VIEWER_PORT)"

visualize-go: build-viewer
	@echo "Starting dependency-free ontology viewer on http://$(VIEWER_HOST):$(VIEWER_PORT)"
	@echo "Default ontology server: $(SERVER_URL)"
	@echo "Press Ctrl+C to stop the viewer."
	$(VIEWER_BINARY) --host "$(VIEWER_HOST)" --port "$(VIEWER_PORT)" --server "$(SERVER_URL)"

visualize-check:
	python3 -m py_compile tools/visualize/server.py
	python3 -m unittest discover -s tools/visualize -p '*_test.py'
	@test -f tools/visualize/index.html || (echo "tools/visualize/index.html is missing" && exit 1)

live-check: build require-entry
	@echo "Checking ontology API at $(SERVER_URL)"
	curl -fsS "$(SERVER_URL)/status" >/tmp/kubernetes-ontology-status.json
	@echo "Checking viewer topology through $(VIEWER_URL)"
	curl -fsS "$(VIEWER_URL)/topology?entityLimit=200&relationLimit=1000" >/tmp/kubernetes-ontology-topology.json
	@echo "Checking viewer diagnostic for Pod $(NAMESPACE)/$(NAME)"
	curl -fsS "$(VIEWER_URL)/diagnostic?kind=Pod&namespace=$(NAMESPACE)&name=$(NAME)&maxDepth=$(MAX_DEPTH)&storageMaxDepth=$(STORAGE_MAX_DEPTH)" >/tmp/kubernetes-ontology-diagnostic.json
	@echo "Live verification passed."

verify-live: live-check

require-kubeconfig:
	@test -n "$(CONFIG)$(KUBECONFIG)" || (echo "CONFIG or KUBECONFIG is required" && exit 1)

require-entry:
	@test -n "$(NAMESPACE)" || (echo "NAMESPACE is required" && exit 1)
	@test -n "$(NAME)" || (echo "NAME is required" && exit 1)
