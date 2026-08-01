import Foundation

@MainActor
final class AIStreamController: ObservableObject {
    let symbol: String
    @Published var configResponse: AIAnalysisConfigResponse?
    @Published var config: AnalysisConfig?
    @Published var status: AIStatus = .idle
    @Published var progress: [AIAgentProgress] = []
    @Published var results: [AIAgentResult] = []
    @Published var narration = ""
    @Published var report = ""
    @Published var decision: AIDecision?
    @Published var error: String?
    @Published var runID: String?
    @Published var lastEventID = 0
    @Published var isLoadingConfig = false

    private var streamTask: Task<Void, Never>?
    private let decoder: JSONDecoder

    init(symbol: String) {
        self.symbol = symbol
        decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
    }

    func loadConfig() async {
        guard configResponse == nil else { return }
        isLoadingConfig = true; defer { isLoadingConfig = false }
        do {
            let response: AIAnalysisConfigResponse = try await APIClient.shared.request("/analysis/ai/config")
            configResponse = response
            if let stored = Self.readStoredConfig(), response.providers.contains(where: { $0.id == stored.llmProvider }) {
                config = stored
            } else if let provider = response.providers.first {
                config = AnalysisConfig(llmProvider: provider.id, deepThinkLlm: provider.models.deep.first ?? "",
                                        quickThinkLlm: provider.models.quick.first ?? "", backendUrl: provider.defaultBackendUrl ?? "",
                                        maxDebateRounds: response.debateRounds.first ?? 1, maxRiskRounds: response.riskRounds.first ?? 1,
                                        onlineTools: true, analysts: response.analysts.filter { $0.defaultEnabled ?? true }.map(\.id))
            }
        } catch { self.error = error.localizedDescription }
    }

    func selectProvider(_ id: String) {
        guard let response = configResponse, let provider = response.providers.first(where: { $0.id == id }), var current = config else { return }
        current.llmProvider = id
        current.deepThinkLlm = provider.models.deep.first ?? ""
        current.quickThinkLlm = provider.models.quick.first ?? ""
        current.backendUrl = provider.defaultBackendUrl ?? ""
        config = current
    }

    func start(research: AnalysisViewModel, replacing: Bool = false) {
        guard let config else { return }
        if replacing { clearOutput() }
        let runID = "run-" + UUID().uuidString.lowercased()
        self.runID = runID
        lastEventID = 0
        Self.store(config)
        connect(config: config, research: research, runID: runID, resume: false)
    }

    func resume(research: AnalysisViewModel) {
        guard let config, let runID else { return }
        connect(config: config, research: research, runID: runID, resume: true)
    }

    func pause() {
        streamTask?.cancel()
        streamTask = nil
        if status == .streaming { status = .paused }
    }

    func writeNLabel(research: AnalysisViewModel) async {
        await research.setNLabel(true, summary: String(report.prefix(1800)))
    }

    func reportFile() throws -> URL {
        let markdown = "# \(symbol) AI 分析报告\n\n> 生成时间：\(ISO8601DateFormatter().string(from: Date()))\n> 来源：CANSLIMAgent 多智能体 AI 分析\n\n\(report)\n"
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("\(symbol)-ai-analysis.md")
        try Data(markdown.utf8).write(to: url, options: .atomic)
        return url
    }

    private func connect(config: AnalysisConfig, research: AnalysisViewModel, runID: String, resume: Bool) {
        streamTask?.cancel()
        status = .streaming
        error = nil
        let context = AIAnalysisContext(
            stockSummary: research.summary, companyProfile: research.profile, secondLayerData: research.secondLayer,
            kline: research.kline.map { .init(symbol: $0.symbol, period: $0.period, source: $0.source, dataAsOf: $0.dataAsOf, recentCandles: Array($0.candles.suffix(120))) },
            rs: research.rs, eps: research.eps, revenue: research.revenue, grossMargin: research.margin,
            chartStatus: ["summary": research.summary == nil ? "error" : "success", "profile": research.profile == nil ? "insufficient" : "success",
                          "kline": research.kline == nil ? "insufficient" : "success", "rs": research.rs == nil ? "insufficient" : "success",
                          "eps": research.eps == nil ? "insufficient" : "success", "revenue": research.revenue == nil ? "insufficient" : "success",
                          "grossMargin": research.margin == nil ? "insufficient" : "success", "secondLayer": research.secondLayer == nil ? "insufficient" : "success"],
            derived: [:])
        let body = AIStartRequest(llmProvider: config.llmProvider, deepThinkLlm: config.deepThinkLlm,
                                  quickThinkLlm: config.quickThinkLlm, backendUrl: config.backendUrl,
                                  maxDebateRounds: config.maxDebateRounds, maxRiskRounds: config.maxRiskRounds,
                                  onlineTools: config.onlineTools, analysts: config.analysts, analysisContext: context,
                                  runId: runID, resume: resume, lastEventId: lastEventID)
        streamTask = Task { [weak self] in
            guard let self else { return }
            do {
                let (bytes, _) = try await APIClient.shared.stream("/analysis/\(symbol)/ai", method: "POST", body: body)
                var block: [String] = []
                for try await line in bytes.lines {
                    try Task.checkCancellation()
                    if line.isEmpty {
                        if let event = SSEParser.parse(block.joined(separator: "\n")) { handle(event) }
                        block.removeAll(keepingCapacity: true)
                    } else { block.append(line) }
                }
                if !block.isEmpty, let event = SSEParser.parse(block.joined(separator: "\n")) { handle(event) }
                if status == .streaming { status = .paused }
            } catch is CancellationError {
                if status == .streaming { status = .paused }
            } catch {
                self.error = readable(error.localizedDescription)
                status = .paused
            }
        }
    }

    private func handle(_ event: SSEEvent) {
        if let id = event.id {
            guard id > lastEventID else { return }
            lastEventID = id
        }
        switch event.event {
        case "heartbeat": break
        case "narration_delta", "thought":
            if let payload = decode(AgentTextPayload.self, event.data) { narration += payload.content }
        case "report_start": narration = ""
        case "report_delta", "report":
            if let payload = decode(ReportPayload.self, event.data) { report += payload.content }
        case "progress":
            guard let payload = decode(ProgressPayload.self, event.data) else { return }
            let agent = payload.agent ?? "AnalysisAgent"
            if let index = progress.firstIndex(where: { $0.agent == agent }) {
                progress[index].status = payload.status ?? "running"; progress[index].message = payload.message ?? ""
            } else { progress.append(.init(agent: agent, status: payload.status ?? "running", message: payload.message ?? "")) }
        case "result":
            if let payload = decode(AgentTextPayload.self, event.data), !payload.content.isEmpty {
                results.append(.init(agent: payload.agent ?? "AnalysisAgent", content: payload.content))
            }
        case "decision": decision = decode(AIDecision.self, event.data)
        case "done":
            status = .done
            streamTask = nil
            Task { [symbol] in
                let _: AIAnalysisMarkResponse? = try? await APIClient.shared.request("/stocks/\(symbol)/ai-analysis", method: "PATCH")
            }
        case "error":
            error = readable(decode(ErrorPayload.self, event.data)?.message ?? "分析中断，请检查 AI 服务配置后重试")
            status = .error
            streamTask = nil
        default: break
        }
    }

    private func decode<T: Decodable>(_ type: T.Type, _ string: String) -> T? { try? decoder.decode(type, from: Data(string.utf8)) }
    private func clearOutput() { progress = []; results = []; narration = ""; report = ""; decision = nil; error = nil }
    private func readable(_ message: String) -> String {
        if message.contains("RestrictedModelsError") || message.contains("Free tier users") { return "当前网关无法访问所选模型，请更换模型后重试。" }
        if message.contains("NoneType") && message.contains("group") { return "模型返回结构解析失败，请更换更稳定的模型后重试。" }
        return message
    }

    private static let configKey = "canslim.ai.config"
    private static func store(_ config: AnalysisConfig) { if let data = try? JSONEncoder().encode(config) { UserDefaults.standard.set(data, forKey: configKey) } }
    private static func readStoredConfig() -> AnalysisConfig? {
        guard let data = UserDefaults.standard.data(forKey: configKey) else { return nil }
        return try? JSONDecoder().decode(AnalysisConfig.self, from: data)
    }
}
