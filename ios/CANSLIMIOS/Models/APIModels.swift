import Foundation

struct APIEnvelope<Value: Decodable>: Decodable {
    let success: Bool
    let data: Value
    let error: String?
}

struct DeletedResponse: Codable { let deleted: Bool }

struct APIFailure: Decodable {
    let error: String?
    let message: String?
}

enum AssetMarket: String, Codable, CaseIterable, Identifiable {
    case us = "US"
    case cn = "CN"
    var id: String { rawValue }
    var title: String { self == .us ? "美股" : "A 股" }
}

enum Market: String, Codable, CaseIterable, Identifiable {
    case sp500 = "SP500"
    case nasdaq100 = "NASDAQ100"
    case russell2000 = "RUSSELL2000"
    case sseMain = "SSE_MAIN"
    case szseMain = "SZSE_MAIN"
    case star = "STAR"
    case chinext = "CHINEXT"
    case bse = "BSE"

    var id: String { rawValue }
    var title: String {
        switch self {
        case .sp500: "标普 500"
        case .nasdaq100: "纳斯达克 100"
        case .russell2000: "罗素 2000"
        case .sseMain: "沪市主板"
        case .szseMain: "深市主板"
        case .star: "科创板"
        case .chinext: "创业板"
        case .bse: "北交所"
        }
    }
    var assetMarket: AssetMarket { [.sp500, .nasdaq100, .russell2000].contains(self) ? .us : .cn }
}

struct Metric<Value: Codable & Equatable>: Codable, Equatable {
    let value: Value
    let sufficient: Bool
}

struct StockMetrics: Codable, Equatable {
    let c: Metric<Double>
    let a: Metric<[Double]>
    let s: Metric<Double>
    let l: Metric<Double>
    let i: Metric<Double>
    let n: Bool?
}

struct Stock: Codable, Identifiable, Equatable, Hashable {
    var id: String { symbol }
    let symbol: String
    let name: String
    let market: Market
    let metrics: StockMetrics
    let aiAnalyzed: Bool
    let dataAsOf: String

    func hash(into hasher: inout Hasher) { hasher.combine(symbol) }
}

struct ThresholdConfig: Codable, Equatable {
    var cMinGrowth: Double = 25
    var aMinGrowth: Double = 25
    var aMinYears: Int = 3
    var sVolumeRatio: Double = 150
    var sMaPeriod: Int = 50
    var lMinRs: Double = 80
    var iMinDelta: Double = 1
}

struct ThresholdPreset: Codable, Identifiable, Equatable {
    let id: Int
    let name: String
    let isDefault: Bool
    let createdAt: String
    let cMinGrowth: Double
    let aMinGrowth: Double
    let aMinYears: Int
    let sVolumeRatio: Double
    let sMaPeriod: Int
    let lMinRs: Double
    let iMinDelta: Double

    var thresholds: ThresholdConfig {
        .init(cMinGrowth: cMinGrowth, aMinGrowth: aMinGrowth, aMinYears: aMinYears,
              sVolumeRatio: sVolumeRatio, sMaPeriod: sMaPeriod, lMinRs: lMinRs, iMinDelta: iMinDelta)
    }
}

struct RefreshJobResponse: Codable { let jobId: String; let status: String }
struct RefreshJob: Codable {
    let id: String
    let market: Market
    let status: String
    let error: String?
    let createdAt: String
    let finishedAt: String?
}

struct PresetInput: Codable {
    let name: String
    let cMinGrowth: Double
    let aMinGrowth: Double
    let aMinYears: Int
    let sVolumeRatio: Double
    let sMaPeriod: Int
    let lMinRs: Double
    let iMinDelta: Double

    init(name: String, thresholds: ThresholdConfig) {
        self.name = name; cMinGrowth = thresholds.cMinGrowth; aMinGrowth = thresholds.aMinGrowth
        aMinYears = thresholds.aMinYears; sVolumeRatio = thresholds.sVolumeRatio; sMaPeriod = thresholds.sMaPeriod
        lMinRs = thresholds.lMinRs; iMinDelta = thresholds.iMinDelta
    }
}

enum ScanWireEvent: Decodable {
    case stock(Stock), status(String), progress(Int, Int), complete(Int, Int), error(String)

    private enum CodingKeys: String, CodingKey { case type, data }
    private struct Progress: Decodable { let analyzed: Int; let total: Int }
    private struct Message: Decodable { let message: String }

    init(from decoder: Decoder) throws {
        let box = try decoder.container(keyedBy: CodingKeys.self)
        switch try box.decode(String.self, forKey: .type) {
        case "stock_result": self = .stock(try box.decode(Stock.self, forKey: .data))
        case "scan_status": self = .status(try box.decode(Message.self, forKey: .data).message)
        case "scan_progress": let value = try box.decode(Progress.self, forKey: .data); self = .progress(value.analyzed, value.total)
        case "scan_complete": let value = try box.decode(Progress.self, forKey: .data); self = .complete(value.analyzed, value.total)
        case "scan_error": self = .error(try box.decode(Message.self, forKey: .data).message)
        default: self = .status("")
        }
    }
}

enum JSONValue: Codable, Equatable {
    case string(String), number(Double), bool(Bool), object([String: JSONValue]), array([JSONValue]), null

    init(from decoder: Decoder) throws {
        let box = try decoder.singleValueContainer()
        if box.decodeNil() { self = .null }
        else if let value = try? box.decode(Bool.self) { self = .bool(value) }
        else if let value = try? box.decode(Double.self) { self = .number(value) }
        else if let value = try? box.decode(String.self) { self = .string(value) }
        else if let value = try? box.decode([String: JSONValue].self) { self = .object(value) }
        else { self = .array(try box.decode([JSONValue].self)) }
    }

    func encode(to encoder: Encoder) throws {
        var box = encoder.singleValueContainer()
        switch self {
        case let .string(value): try box.encode(value)
        case let .number(value): try box.encode(value)
        case let .bool(value): try box.encode(value)
        case let .object(value): try box.encode(value)
        case let .array(value): try box.encode(value)
        case .null: try box.encodeNil()
        }
    }
}

enum KLinePeriod: String, Codable, CaseIterable, Identifiable {
    case daily, weekly, monthly
    var id: String { rawValue }
    var title: String { self == .daily ? "日" : self == .weekly ? "周" : "月" }
}

struct MetricSource: Codable, Equatable {
    let source: String
    let status: String
}

struct StockSummary: Codable, Equatable {
    let symbol: String
    let name: String
    let market: String
    let metrics: StockMetrics
    let aiAnalyzed: Bool
    let dataAsOf: String
    let nAnalysisAt: String?
    let nAnalysisSummary: String
    let metricSources: [String: MetricSource]?
}

struct CompanyProfile: Codable, Equatable {
    let symbol: String
    let name: String
    let description: String
    let sector: String
    let industry: String
    let website: String
    let source: String
    let updatedAt: String
}

struct Candle: Codable, Identifiable, Equatable {
    var id: String { date }
    let date: String
    let open: Double
    let high: Double
    let low: Double
    let close: Double
    let volume: Double
}

struct KLineData: Codable, Equatable {
    let symbol: String
    let period: KLinePeriod
    let source: String
    let dataAsOf: String
    let candles: [Candle]
}

struct RSPoint: Codable, Identifiable, Equatable {
    var id: String { date }
    let date: String
    let value: Double
}

struct RSData: Codable, Equatable {
    let symbol: String
    let period: KLinePeriod
    let source: String
    let dataAsOf: String
    let rsLine: [RSPoint]
}

struct EPSQuarter: Codable, Identifiable, Equatable {
    var id: String { period }
    let period: String
    let eps: Double
    let epsYoyGrowth: Double
    let dataSufficient: Bool
    let reportDate: String
}

struct EPSData: Codable, Equatable {
    let symbol: String
    let source: String
    let dataAsOf: String
    let quarters: [EPSQuarter]
}

struct RevenueYear: Codable, Identifiable, Equatable {
    var id: Int { year }
    let year: Int
    let revenue: Double
    let revenueYoyGrowth: Double
    let dataSufficient: Bool
}

struct RevenueData: Codable, Equatable {
    let symbol: String
    let source: String
    let dataAsOf: String
    let years: [RevenueYear]
}

struct GrossMarginQuarter: Codable, Identifiable, Equatable {
    var id: String { period }
    let period: String
    let grossMargin: Double
    let reportDate: String
}

struct GrossMarginData: Codable, Equatable {
    let symbol: String
    let source: String
    let dataAsOf: String
    let quarters: [GrossMarginQuarter]
}

struct NLabelResponse: Codable {
    let symbol: String
    let nLabel: Bool
    let nAnalysisAt: String
}

enum TradePlanDirection: String, Codable, CaseIterable, Identifiable {
    case buy, sell, watch
    var id: String { rawValue }
    var title: String { self == .buy ? "买入" : self == .sell ? "卖出" : "观察" }
}

enum TradePlanStatus: String, Codable, CaseIterable, Identifiable {
    case draft, waiting, executed, cancelled, expired
    var id: String { rawValue }
    var title: String {
        switch self {
        case .draft: "草稿"
        case .waiting: "等待触发"
        case .executed: "已执行"
        case .cancelled: "已取消"
        case .expired: "已过期"
        }
    }
}

struct TradePlan: Codable, Identifiable, Equatable, Hashable {
    let id: String
    let ticker: String
    let direction: TradePlanDirection
    let status: TradePlanStatus
    let planDate: String
    let validUntil: String?
    let entryTrigger: String?
    let entryPrice: Double?
    let stopLoss: Double?
    let takeProfit: Double?
    let positionSize: Double?
    let rationale: String?
    let aiSummary: String?
    let notes: String?
    let executedJournalId: String?
    let executedAt: String?
    let createdAt: String
    let updatedAt: String

    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}

struct TradePlanInput: Codable {
    let ticker: String
    let direction: TradePlanDirection
    let status: TradePlanStatus
    let planDate: String
    let validUntil: String?
    let entryTrigger: String?
    let entryPrice: Double?
    let stopLoss: Double?
    let takeProfit: Double?
    let positionSize: Double?
    let rationale: String?
    let aiSummary: String?
    let notes: String?
}

enum TradeSide: String, Codable, CaseIterable, Identifiable {
    case buy, sell
    var id: String { rawValue }
    var title: String { self == .buy ? "买入" : "卖出" }
}

enum TradeMarket: String, Codable, CaseIterable, Identifiable {
    case us, hk, chinaA = "china_a", crypto, other
    var id: String { rawValue }
    var title: String {
        switch self {
        case .us: "美股"
        case .hk: "港股"
        case .chinaA: "A 股"
        case .crypto: "加密资产"
        case .other: "其他"
        }
    }
}

struct JournalEntry: Codable, Identifiable, Equatable, Hashable {
    let id: String
    let datetime: String
    let symbol: String
    let name: String
    let side: TradeSide
    let quantity: Double
    let price: Double
    let amount: Double
    let fee: Double
    let market: TradeMarket
    let currency: String
    let orderId: String?
    let source: String
    let notes: String?
    let reflection: String?
    let createdAt: String
    let updatedAt: String

    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}

struct JournalInput: Codable {
    let datetime: String
    let symbol: String
    let name: String?
    let side: TradeSide
    let quantity: Double
    let price: Double
    let amount: Double?
    let fee: Double?
    let market: TradeMarket
    let currency: String?
    let orderId: String?
    let notes: String?
}

struct JournalPatch: Codable { let notes: String?; let reflection: String? }

struct StoredJournalReflection: Codable, Identifiable {
    let id: String
    let createdAt: String
    let scope: String
    let recordCount: Int
    let engine: String
    let payload: [String: JSONValue]
}

struct ExecuteTradePlanInput: Codable {
    let datetime: String
    let quantity: Double
    let price: Double
    let market: TradeMarket
    let name: String?
    let fee: Double?
    let currency: String?
    let orderId: String?
    let notes: String?
}

struct ExecuteTradePlanResponse: Codable { let plan: TradePlan; let journal: JournalEntry }

enum BrokerID: String, Codable, CaseIterable, Identifiable {
    case longport, tiger, futu, ibkr
    var id: String { rawValue }
    var title: String {
        switch self { case .longport: "长桥"; case .tiger: "老虎证券"; case .futu: "富途 OpenD"; case .ibkr: "IBKR" }
    }
}

struct BrokerConnection: Codable, Identifiable, Equatable {
    let id: String
    let broker: BrokerID
    let label: String
    let accountName: String
    let connectionMethod: String
    let connectionStatus: String
    let message: String
    let connected: Bool
    let secureStorageReady: Bool
    let lastSyncedAt: String?
}

struct BrokerConnectionStart: Codable {
    let action: String?
    let authorizationUrl: String?
    let id: String?
    let broker: BrokerID?
    let label: String?
    let accountName: String?
    let connectionMethod: String?
    let connectionStatus: String?
    let message: String?
    let connected: Bool?
    let secureStorageReady: Bool?
    let lastSyncedAt: String?
}

struct BrokerPosition: Codable, Identifiable {
    var id: String { symbol }
    let symbol: String
    let name: String
    let quantity: Double
    let market: String
    let currency: String
}

struct BrokerExecution: Codable, Identifiable, Hashable {
    var id: String { externalId }
    let externalId: String
    let symbol: String
    let name: String
    let side: String
    let quantity: Double
    let price: Double
    let fee: Double
    let currency: String
    let market: String
    let executedAt: String
}

struct BrokerSyncResult: Codable { let positions: [BrokerPosition]; let executions: [BrokerExecution]; let warnings: [String] }
struct BrokerSyncInput: Codable { let startAt: String?; let endAt: String? }
struct BrokerImportInput: Codable { let executions: [BrokerExecution] }
struct BrokerImportResponse: Codable { let created: Int; let skipped: Int; let warnings: [String] }
