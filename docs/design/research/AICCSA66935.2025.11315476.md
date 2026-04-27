# Kubernetes 集群实时语义建模与 Ontology 数据库调研

*更新日期：2026-04-21*

## 摘要

本文档调研当前 Kubernetes 集群内信息实时建模到 ontology / semantic database / graph database 的主流方法，并结合论文 **Ontological Modeling of Kubernetes Clusters Leveraging In-Memory Dependency Graphs**（AICCSA 2025, DOI: 10.1109/AICCSA66935.2025.11315476）的全文内容，提炼出适合 `kubernetes-ontology` 项目的实现路线。

调研结论是：

> 当前业界最可行的路线不是“纯 ontology-first”，而是 **in-memory dependency graph + incremental ontology syncing + rule-based reasoning** 的混合架构。

也就是说，实时性来自：

- Kubernetes API / watch / informer
- 内存依赖图
- 增量更新

而语义能力来自：

- RDF / OWL ontology
- object/data properties
- OWL RL reasoning
- SWRL rules
- SPARQL queries

这种混合方案既保留了图结构对动态依赖关系的高效表示能力，也具备 ontology 在一致性约束、语义推理、复杂查询和智能监督方面的优势。

---

## 1. 调研目标

本文档关注的问题是：

1. 当前业界如何对 Kubernetes 集群进行实时建模？
2. ontology / semantic model 在 Kubernetes 中的实际作用是什么？
3. graph database 与 ontology database 的边界如何划分？
4. 如果要实现一个 `kubernetes-ontology` 项目，推荐的架构应是什么？

---

## 2. 问题背景

Kubernetes 集群具有典型的高动态、强依赖、跨层级结构：

- Node、Pod、Container、Service、Deployment、ReplicaSet、StatefulSet
- ConfigMap、Secret、PVC、PV、NetworkPolicy、Event
- 控制面关系、调度关系、服务发现关系、配置依赖、运行时调用关系

这带来几个困难：

- 集群状态变化频繁，不能依赖静态配置建模
- 资源间依赖分散在多个 API 对象中
- 需要同时支持结构查询、关系查询、推理和告警
- 传统 YAML/JSON 级别查询缺少语义表达能力

因此，若想实现更高级的：

- observability
- policy enforcement
- drift detection
- fault diagnosis
- intelligent automation

就需要在“运行态依赖图”之上叠加“语义层”。

---

## 3. 业界主流技术路线

## 3.1 路线 A：实时拓扑图 / 实体图

这是当前业界最成熟的形态。

### 典型代表
- Dynatrace Smartscape
- Datadog Service Map
- New Relic Service Maps / Entity Graph
- Kiali Service Graph
- OpenTelemetry service graph connector

### 核心特征
- 自动识别实体：service、pod、node、process、application
- 建立依赖边：calls、runs_on、targets、owns、belongs_to
- 持续更新
- 面向可视化、根因分析、影响分析

### 优点
- 实时性强
- 对运行态关系支持最好
- 工程落地成熟

### 局限
- 语义约束弱
- 缺少本体级推理
- 对高层业务语义支撑有限

### 启发
这条路线说明：

> 工业界首先解决的是“运行态图”，而不是“形式语义完备”。

---

## 3.2 路线 B：Property Graph / Knowledge Graph

很多平台会把 Kubernetes、微服务、CMDB、告警、链路信息统一进图数据库。

### 常见实现
- Neo4j
- Amazon Neptune
- JanusGraph
- TigerGraph
- 自建 topology graph store

### 用途
- blast radius analysis
- root cause analysis
- ownership graph
- dependency traversal
- policy impact analysis

### 优点
- 图遍历自然
- 多跳关系分析方便
- 增量更新容易

### 局限
- 形式语义能力不如 OWL
- schema/constraint 约束弱
- 自动推理能力有限

### 启发
这条路线适合作为 ontology 的下层运行时基础设施。

---

## 3.3 路线 C：Ontology / Semantic Web

这一路线在研究和部分高治理需求场景中出现得更多。

### 特征
- 使用 RDF / OWL 表达资源、关系、属性
- 定义 classes、object properties、data properties
- 使用 reasoner 执行逻辑一致性验证
- 用 SPARQL 做复杂查询
- 用 SWRL / OWL RL 做规则推理

### 优点
- 语义表达能力强
- 便于定义约束和不变量
- 适合自动推理和高级监督

### 局限
- 工程复杂度高
- 全量更新成本高
- 对高频动态状态直接建模并不经济

### 启发
ontology 更适合作为：

- semantic layer
- rule layer
- consistency layer

而不是直接承担高频运行态同步的全部职责。

---

## 3.4 路线 D：Catalog / 元数据目录

如 Backstage catalog 这类系统更偏向“组织语义建模”。

### 用途
- 服务目录
- team/system/domain ownership
- API 与 component 关系

### 启发
这类模型适合和 Kubernetes ontology 的高层业务语义对接，但不适合作为实时依赖图主干。

---

## 4. 论文全文精读：AICCSA 2025 方法提炼

本文档重点参考论文：

**Jridi, A., Kachroudi, M., Zaharia, M., & Farah, M. (2025). Ontological Modeling of Kubernetes Clusters Leveraging In-Memory Dependency Graphs.**

这篇论文给出了一个非常接近工程可落地的方案。

---

## 4.1 论文要解决的问题

作者指出，现有方案不能同时很好支持：

- Kubernetes 集群的动态变化
- 多层资源的完整语义表示
- 依赖关系建模
- 一致性检查
- 推理与高级查询

因此，论文提出了一种：

> **基于内存依赖图驱动的动态 ontology 建模方法。**

---

## 4.2 总体架构

论文的方法是一个 hybrid workflow：

```text
Kubernetes API
    ↓
Resource taxonomy initialization
    ↓
In-memory Directed Dependency Graph (DDG)
    ↓
RDF/OWL ontology generation and sync
    ↓
OWL RL reasoning + SWRL rules
    ↓
SPARQL querying + semantic supervision
```

其核心思想是：

- **运行态事实先进入 DDG**
- **再从 DDG 增量同步到 ontology**
- **最后在 ontology 上做规则推理与语义查询**

---

## 4.3 第一步：资源类型初始化与语义分类

论文首先初始化 Kubernetes API 各资源 client，并定义 semantic taxonomy。

它明确说明：

- 涉及 core、application、storage、networking、policy 等资源组
- 原生资源和 CRD 都被纳入类型体系
- 这个阶段仅加载轻量级 client handle 和结构定义，因此内存占用较小

### 启发
你的项目应先建立：

- resource kind registry
- semantic type hierarchy
- API client abstraction

而不是上来就构建 triples。

---

## 4.4 第二步：构建 DDG 与 Ontology 两个结构

### DDG 定义
论文定义 Directed Dependency Graph：

\[
G = (V, E, \phi)
\]

其中：

- `V`：Kubernetes 资源节点集合
- `E`：依赖边集合
- `ϕ`：边语义标签函数

### 节点属性
每个 vertex 至少包含：

- `id`
- `kind`
- `name`
- `namespace`
- `labels`
- `annotations`

### 边类型
论文给出的关系类型包括：

- `scheduledOn`
- `usesConfigMap`
- `usesSecret`
- `owns`
- `targets`
- `runsOn`
- `mountedOn`
- `controlledBy`

### Ontology 定义
论文同时定义 ontology：

\[
O = (C, P, I)
\]

其中：

- `C`：classes
- `P`：properties
- `I`：instances

#### classes 示例
- Pod
- Service
- Node
- Namespace
- ConfigMap
- Secret

#### properties 分类
- object properties：`scheduledOn`, `usesConfigMap`, `targets`, `owns`
- data properties：`hasCPUUsage`, `hasMemoryLimit`, `hasStatus`

### 关键启发
这是全文最关键的一点：

> **DDG 是运行态结构层，ontology 是语义表达层。**

---

## 4.5 第三步：依赖图构建算法

论文给出了显式算法（Algorithm 1）。

### 算法流程
1. `API.getAllResources(K)` 获取全部资源
2. 为每个 resource 创建 vertex
3. `API.getDependencies(resource)` 获取依赖
4. 为依赖对象创建或复用 vertex
5. 建立 edge
6. 用 `determineRelationType(resource, dep)` 确定边类型

### 动态更新定义
论文把图更新写成：

\[
G_{t+\Delta t} = G_t \oplus \Delta(Resources_{t+\Delta t}, Resources_t)
\]

即：

- 比较 cluster state 差异
- 对 graph 做增删改
- 避免重建全图

### 启发
这为你的项目直接提供了更新策略：

- 基于 informer/watch 维护 live state
- diff state
- incrementally mutate graph

---

## 4.6 第四步：Ontology 同步机制

论文定义同步函数：

\[
S : G \rightarrow O
\]

用于保证：

- 每个 graph node 对应唯一 ontology instance
- 每条 graph edge 对应 ontology triple

### 三个约束

#### Unique Instance Constraint
每个节点唯一映射到一个 ontology instance。

#### Edge-to-Property Mapping Constraint
每条 dependency edge 映射到对应 object property。

#### No Dangling Reference Constraint
ontology 中不能出现悬空引用。

### 同步策略
论文强调 ontology syncing 是**incremental**：

- 新资源 -> 新实例
- 删除资源 -> 删除实例及其 triples
- 新依赖 -> 插入 triples
- 消失依赖 -> 删除 triples

### 复杂度
作者认为复杂度可以近似为：

\[
O(n)
\]

其中 `n` 是变更的 vertices/edges 数量。

### 启发
你的项目里 ontology syncing 应该是一个独立模块，而不是散落在各个 collector 中。

---

## 4.7 第五步：规则推理与语义查询

论文使用：

- **OWL RL reasoning**
- **SWRL rules**
- **Pellet reasoner**
- **SPARQL**
- **RDFLib**
- **owlready2**

其目标包括：

- infer implicit knowledge
- identify inconsistent states
- enable semantic supervision

也就是说 ontology 的用途不是静态描述，而是：

- runtime diagnostics
- alert triggering
- policy checking
- intelligent supervision

---

## 4.8 Event 建模示例

论文给出一个 Event ontology fragment，对一个 Pod 事件建模，包括：

- event type
- reason = Pulled
- reporting component = kubelet
- involved object = Pod
- timestamp
- message
- reportsOn 关系

### 启发
这说明强 observability ontology 不能只建资源图，还应把：

- Event
- Alert
- Reporting source
- Timestamp
- Reason

纳入模型。

---

## 5. 论文实验结果

## 5.1 查询性能

论文比较了 ontology SPARQL 查询与原生 Kubernetes API 调用：

| 查询 | SPARQL(ms) | K8s API(ms) |
|---|---:|---:|
| List Pods in Namespace X | 45 | 40 |
| List all Services | 30 | 28 |
| ReplicaSets and managed Pod count | 61 | 52 |
| Total memory capacity of Nodes | 80 | 78 |

### 结论
- 查询性能接近原生 API
- 准确率达到 100%
- ontology 层未带来不可接受的延迟

---

## 5.2 本体质量指标

论文使用两个指标：

### Relationship Richness (RR)
\[
RR = \frac{|P_o|}{|P_o| + |P_d|}
\]

结果：
- **RR = 0.83**

### Attribute Richness (AR)
\[
AR = \frac{\sum_{c \in C}|D(c)|}{|C|}
\]

结果：
- **AR = 0.36**

论文指出，这两个值高于 HOCC 文献中的：
- RR = 0.75
- AR = 0.30

说明其 ontology 对关系和属性都建模得更充分。

---

## 5.3 推理能力

论文使用 20 条 SWRL 规则，文中展示了 5 个代表场景：

| 场景 | 正确推理 | FP | FN | 时间(s) |
|---|---:|---:|---:|---:|
| High CPU usage triggers alert | 3/3 | 0 | 0 | 1.2 |
| Orphaned Pod | 2/2 | 0 | 0 | 1.1 |
| Unbound PVC | 1/1 | 0 | 0 | 1.0 |
| Excessive Pod restarts | 1/1 | 0 | 0 | 1.3 |
| NetworkPolicy not applied to any Pod | 2/2 | 0 | 0 | 1.1 |

### 结论
- 正确率 100%
- false positive / false negative 都为 0
- 平均推理时间约 1.1 秒

### 启发
这证明 ontology 层可以承担实际运维能力：

- orphaned resource detection
- storage alerting
- policy checking
- restart instability detection
- high CPU alert reasoning

---

## 6. 结合全文后的设计结论

基于业界实践与论文全文，可以得出一个很清晰的结论：

> **最合理的 Kubernetes ontology 架构不是直接“YAML → RDF”，而是“API / telemetry → in-memory dependency graph → incremental ontology sync → reasoning / SPARQL”。**

这也是本文最重要的结论。

---

## 7. 对 `kubernetes-ontology` 项目的建议架构

## 7.1 双层模型

### Layer 1：Operational Graph
用于高频、增量、实时更新。

#### 职责
- 接入 Kubernetes API / informer / watch
- 维护 resource nodes
- 维护 dependency edges
- 处理 state diff
- 提供快速结构查询

### Layer 2：Semantic Ontology
用于约束、语义抽象和规则推理。

#### 职责
- 维护 classes / properties / instances
- 维护 graph-to-ontology mapping
- 执行 OWL RL / SWRL reasoning
- 提供 SPARQL 查询
- 支持 alerts / policy checks / diagnostics

---

## 7.2 建议优先建模的资源

第一阶段：

- Cluster
- Namespace
- Node
- Pod
- Container
- Deployment
- ReplicaSet
- StatefulSet
- Service
- ConfigMap
- Secret
- PVC
- PV
- Ingress
- Event

第二阶段：

- NetworkPolicy
- ServiceAccount
- Role / RoleBinding
- HPA
- Job / CronJob
- Alert
- MetricsSummary
- Trace / Span

---

## 7.3 建议优先建模的关系

- `scheduledOn`
- `targets`
- `owns`
- `controlledBy`
- `usesConfigMap`
- `usesSecret`
- `mountedOn`
- `runsOn`
- `reportsOn`
- `belongsToNamespace`
- `partOf`
- `dependsOn`

同时区分两类关系：

- **spec-derived relation**：从配置推导
- **runtime-observed relation**：从事件 / 指标 / 链路观察

---

## 7.4 建议模块划分

```text
collector/
  kubernetes_api/
  informer/
  event_stream/

graph/
  vertex_model/
  edge_model/
  dependency_resolver/
  incremental_updater/

ontology/
  class_registry/
  property_registry/
  instance_mapper/
  sync_engine/
  triple_store/

reasoning/
  owl_rl/
  swrl_rules/
  policy_checks/

query/
  graph_query/
  sparql_query/

examples/
  events/
  alerts/
  orphaned_pods/
  pvc_binding/
```

---

## 7.5 建议实现顺序

### Phase 1：Dependency Graph
- 完成 API collector
- 完成资源节点与依赖边抽取
- 完成 graph diff / incremental update

### Phase 2：Ontology Schema
- 定义 classes
- 定义 object/data properties
- 建立 graph-to-instance mapping

### Phase 3：Ontology Sync
- 实现实例生成
- 实现 triple 增删改
- 保证 unique instance / no dangling refs

### Phase 4：Reasoning & Query
- 接入 reasoner
- 实现 SWRL rules
- 实现 SPARQL examples

### Phase 5：Operational Use Cases
- orphaned pod detection
- unbound PVC detection
- node CPU alert inference
- network policy misconfiguration detection
- event-to-resource correlation

---

## 8. 最终结论

当前 Kubernetes 语义建模最值得采用的方案是：

> **以 Directed Dependency Graph 为运行态基础，以 RDF/OWL ontology 为语义层，以增量同步为连接机制，以 SWRL/OWL RL/SPARQL 为推理和查询工具。**

AICCSA 2025 论文的价值在于，它不是停留在抽象 ontology 设计，而是清楚回答了下面这些工程问题：

- 如何从 Kubernetes API 获取 live state
- 如何表示依赖关系
- 如何做增量 graph update
- 如何同步到 ontology
- 如何做 reasoning 与 alerting
- 如何验证性能和准确率

对 `kubernetes-ontology` 项目而言，这篇论文提供了一个非常直接的设计模板。

---

## 参考资料

1. Jridi, A., Kachroudi, M., Zaharia, M., & Farah, M. (2025). *Ontological Modeling of Kubernetes Clusters Leveraging In-Memory Dependency Graphs*. 2025 IEEE/ACS 22nd International Conference on Computer Systems and Applications (AICCSA). https://doi.org/10.1109/AICCSA66935.2025.11315476
2. Dynatrace. *Smartscape*. https://docs.dynatrace.com/docs/discover-dynatrace/platform/smartscape
3. Datadog. *Service Map*. https://docs.datadoghq.com/tracing/services/services_map/
4. New Relic. *Service map: Visualize your entities*. https://docs.newrelic.com/docs/new-relic-solutions/new-relic-one/ui-data/service-maps/service-maps/
5. Backstage. *Descriptor Format of Catalog Entities / Relations*. https://backstage.io/docs/features/software-catalog/descriptor-format#relations
6. OpenTelemetry documentation. https://opentelemetry.io/
7. Kiali documentation. https://kiali.io/
