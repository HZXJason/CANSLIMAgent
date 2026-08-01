import Foundation

struct AIProviderModels: Codable, Equatable { let deep: [String]; let quick: [String] }

struct AIProvider: Codable, Identifiable, Equatable {
    let id: String
    let name: String
    let models: AIProviderModels
    let customModels: Bool?
    let defaultBackendUrl: String?
}

struct AIAnalyst: Codable, Identifiable, Equatable {
    let id: String
    let name: String
    let description: String
    let defaultEnabled: Bool?
}

struct AIAnalysisConfigResponse: Codable {
    let providers: [AIProvider]
    let analysts: [AIAnalyst]
    let debateRounds: [Int]
    let riskRounds: [Int]
}

struct AnalysisConfig: Codable, Equatable {
    var llmProvider: String
    var deepThinkLlm: String
    var quickThinkLlm: String
    var backendUrl: String
    var maxDebateRounds: Int
    var maxRiskRounds: Int
    var onlineTools: Bool
    var analysts: [String]
}

struct AIKLineContext: Codable {
    let symbol: String
    let period: KLinePeriod
    let source: String
    let dataAsOf: String
    let recentCandles: [Candle]
}

struct AIAnalysisContext: Codable {
    let stockSummary: StockSummary?
    let companyProfile: CompanyProfile?
    let secondLayerData: [String: JSONValue]?
    let kline: AIKLineContext?
    let rs: RSData?
    let eps: EPSData?
    let revenue: RevenueData?
    let grossMargin: GrossMarginData?
    let chartStatus: [String: String]
    let derived: [String: JSONValue]
}

struct AIStartRequest: Codable {
    let llmProvider: String
    let deepThinkLlm: String
    let quickThinkLlm: String
    let backendUrl: String
    let maxDebateRounds: Int
    let maxRiskRounds: Int
    let onlineTools: Bool
    let analysts: [String]
    let analysisContext: AIAnalysisContext
    let runId: String
    let resume: Bool
    let lastEventId: Int
}

enum AIStatus: String { case idle, streaming, paused, done, error }

struct AIAgentProgress: Identifiable, Equatable {
    var id: String { agent }
    let agent: String
    var status: String
    var message: String
}

struct AIAgentResult: Identifiable, Equatable {
    let id = UUID()
    let agent: String
    let content: String
}

struct AIDecision: Codable, Equatable {
    let action: String
    let confidence: String
    let summary: String
}

struct SSEEvent {
    let event: String
    let data: String
    let id: Int?
}

enum SSEParser {
    static func parse(_ block: String) -> SSEEvent? {
        var event = "message"
        var id: Int?
        var data: [String] = []
        for line in block.components(separatedBy: .newlines) {
            if line.hasPrefix("event:") { event = String(line.dropFirst(6)).trimmingCharacters(in: .whitespaces) }
            else if line.hasPrefix("id:") { id = Int(String(line.dropFirst(3)).trimmingCharacters(in: .whitespaces)) }
            else if line.hasPrefix("data:") { data.append(String(line.dropFirst(5)).trimmingCharacters(in: .whitespaces)) }
        }
        return data.isEmpty && event == "message" ? nil : SSEEvent(event: event, data: data.joined(separator: "\n"), id: id)
    }
}

struct AgentTextPayload: Codable { let agent: String?; let content: String }
struct ProgressPayload: Codable { let agent: String?; let status: String?; let message: String? }
struct ReportPayload: Codable { let content: String }
struct ErrorPayload: Codable { let message: String }
struct AIAnalysisMarkResponse: Codable { let symbol: String; let aiAnalyzed: Bool; let aiAnalysisAt: String }

struct AIModelDiscoveryResponse: Codable { let models: [String]; let source: String }
struct AIModelDiscoveryInput: Codable { let provider: String; let backendUrl: String? }
struct LLMCredentialStatus: Codable {
    let provider: String
    let backendHost: String
    let configured: Bool
    let storageReady: Bool
    let updatedAt: String?
}
struct LLMCredentialInput: Codable { let apiKey: String; let backendUrl: String }
