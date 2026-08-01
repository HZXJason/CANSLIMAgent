import Charts
import SwiftUI

struct AnalysisHomeView: View {
    @Binding var initialSymbol: String
    @Binding var initialSection: String
    @State private var input = ""
    @State private var path: [String] = []

    var body: some View {
        NavigationStack(path: $path) {
            ContentUnavailableView {
                Label("个股分析", systemImage: "chart.xyaxis.line")
            } description: {
                Text("输入股票代码，查看研究证据和 AI 多智能体分析。")
            } actions: {
                HStack {
                    TextField(SettingsStore.shared.assetMarket == .us ? "NVDA" : "600519.SH", text: $input)
                        .textInputAutocapitalization(.characters).autocorrectionDisabled().textFieldStyle(.roundedBorder)
                    Button("打开") { open(input) }.buttonStyle(.borderedProminent).disabled(input.trimmingCharacters(in: .whitespaces).isEmpty)
                }
                .frame(maxWidth: 340)
            }
            .navigationTitle("分析")
            .navigationDestination(for: String.self) { symbol in AnalysisDetailView(symbol: symbol, initialSection: initialSection) }
            .onChange(of: initialSymbol) { _, value in
                guard !value.isEmpty else { return }
                open(value)
                initialSymbol = ""
            }
            .onAppear {
                if !initialSymbol.isEmpty { open(initialSymbol); initialSymbol = "" }
            }
        }
    }

    private func open(_ symbol: String) {
        let normalized = symbol.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        guard !normalized.isEmpty else { return }
        input = ""
        path.append(normalized)
    }
}

@MainActor
final class AnalysisViewModel: ObservableObject {
    let symbol: String
    @Published var summary: StockSummary?
    @Published var profile: CompanyProfile?
    @Published var kline: KLineData?
    @Published var rs: RSData?
    @Published var eps: EPSData?
    @Published var revenue: RevenueData?
    @Published var margin: GrossMarginData?
    @Published var secondLayer: [String: JSONValue]?
    @Published var period: KLinePeriod = .daily
    @Published var isLoading = false
    @Published var error: String?

    init(symbol: String) { self.symbol = symbol }

    func load() async {
        isLoading = true
        error = nil
        async let summaryResult: Result<StockSummary, Error> = capture { try await APIClient.shared.request("/stocks/\(encodedSymbol)/summary") }
        async let profileResult: Result<CompanyProfile, Error> = capture { try await APIClient.shared.request("/stocks/\(encodedSymbol)/profile") }
        async let klineResult: Result<KLineData, Error> = capture { try await chartRequest("kline") }
        async let rsResult: Result<RSData, Error> = capture { try await chartRequest("rs") }
        async let epsResult: Result<EPSData, Error> = capture { try await APIClient.shared.request("/stocks/\(encodedSymbol)/eps/quarterly") }
        async let revenueResult: Result<RevenueData, Error> = capture { try await APIClient.shared.request("/stocks/\(encodedSymbol)/revenue/annual") }
        async let marginResult: Result<GrossMarginData, Error> = capture { try await APIClient.shared.request("/stocks/\(encodedSymbol)/margin/gross/quarterly") }
        async let secondResult: Result<[String: JSONValue], Error> = capture { try await APIClient.shared.request("/stocks/\(encodedSymbol)/second-layer-data") }

        let firstResult = await summaryResult
        summary = firstResult.value
        if summary == nil { error = firstResult.failure ?? "无法读取个股数据" }
        isLoading = false

        let remaining = await (profileResult, klineResult, rsResult, epsResult, revenueResult, marginResult, secondResult)
        profile = remaining.0.value
        kline = remaining.1.value
        rs = remaining.2.value
        eps = remaining.3.value
        revenue = remaining.4.value
        margin = remaining.5.value
        secondLayer = remaining.6.value
    }

    func changePeriod(_ value: KLinePeriod) async {
        period = value
        async let newKLine: KLineData = chartRequest("kline")
        async let newRS: RSData = chartRequest("rs")
        do { (kline, rs) = try await (newKLine, newRS) }
        catch { self.error = error.localizedDescription }
    }

    func setNLabel(_ enabled: Bool, summary analysisSummary: String) async {
        do {
            let body = NLabelUpdate(nLabel: enabled, analysisSummary: analysisSummary)
            let _: NLabelResponse = try await APIClient.shared.request("/stocks/\(encodedSymbol)/n-label", method: "PATCH", body: body)
            await load()
        } catch { self.error = error.localizedDescription }
    }

    private var encodedSymbol: String { symbol.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? symbol }
    private func chartRequest<T: Decodable>(_ endpoint: String) async throws -> T {
        try await APIClient.shared.request("/stocks/\(encodedSymbol)/\(endpoint)", query: [.init(name: "period", value: period.rawValue)])
    }
    private func capture<T>(_ operation: () async throws -> T) async -> Result<T, Error> {
        do { return .success(try await operation()) } catch { return .failure(error) }
    }
}

private struct NLabelUpdate: Encodable { let nLabel: Bool; let analysisSummary: String }

private extension Result {
    var value: Success? { try? get() }
    var failure: String? {
        guard case let .failure(error) = self else { return nil }
        return error.localizedDescription
    }
}

struct AnalysisDetailView: View {
    let symbol: String
    @StateObject private var model: AnalysisViewModel
    @StateObject private var ai: AIStreamController
    @StateObject private var tradePlans = TradePlansViewModel()
    @State private var section = 0
    @State private var showsNLabel = false
    @State private var showsTradePlan = false
    @State private var nSummary = ""

    init(symbol: String, initialSection: String = "overview") {
        self.symbol = symbol
        _model = StateObject(wrappedValue: AnalysisViewModel(symbol: symbol))
        _ai = StateObject(wrappedValue: AIStreamController(symbol: symbol))
        _section = State(initialValue: initialSection == "charts" ? 1 : initialSection == "ai" ? 2 : 0)
    }

    var body: some View {
        Group {
            if model.isLoading && model.summary == nil {
                LoadingStateView(label: "正在汇总 \(symbol) 的研究证据")
            } else if let error = model.error, model.summary == nil {
                ErrorStateView(message: error) { Task { await model.load() } }
            } else {
                ScrollView {
                    LazyVStack(spacing: 14) {
                        header
                        Picker("分析区域", selection: $section) {
                            Text("概览").tag(0)
                            Text("图表").tag(1)
                            Text("AI").tag(2)
                        }
                        .pickerStyle(.segmented)
                        switch section {
                        case 0: overview
                        case 1: charts
                        default: AIAnalysisView(symbol: symbol, research: model, controller: ai)
                        }
                    }.padding()
                }
                .refreshable { await model.load() }
            }
        }
        .navigationTitle(symbol)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { showsTradePlan = true } label: { Label("新建交易计划", systemImage: "doc.badge.plus") }
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button { showsNLabel = true } label: { Label("更新 N 标签", systemImage: "n.circle") }
            }
        }
        .task { await model.load() }
        .sheet(isPresented: $showsTradePlan) {
            TradePlanEditor(
                model: tradePlans,
                defaultTicker: symbol,
                defaultAISummary: ai.decision?.summary ?? ai.report
            )
        }
        .alert("更新 N 标签", isPresented: $showsNLabel) {
            TextField("新产品、新高或新事件摘要", text: $nSummary)
            Button("标记通过") { Task { await model.setNLabel(true, summary: nSummary) } }
            Button("标记不通过", role: .destructive) { Task { await model.setNLabel(false, summary: nSummary) } }
            Button("取消", role: .cancel) {}
        } message: { Text("该结论会同步到 Web 端选股结果。") }
    }

    @ViewBuilder private var header: some View {
        if let summary = model.summary {
            ResearchCard(title: summary.name.isEmpty ? symbol : summary.name, subtitle: "\(summary.market) · 数据截至 \(summary.dataAsOf)") {
                HStack(spacing: 8) {
                    SummaryChip(label: "C", value: summary.metrics.c.value, sufficient: summary.metrics.c.sufficient)
                    SummaryChip(label: "S", value: summary.metrics.s.value, sufficient: summary.metrics.s.sufficient)
                    SummaryChip(label: "L", value: summary.metrics.l.value, sufficient: summary.metrics.l.sufficient)
                    SummaryChip(label: "I", value: summary.metrics.i.value, sufficient: summary.metrics.i.sufficient)
                }
                if !summary.nAnalysisSummary.isEmpty {
                    Label(summary.nAnalysisSummary, systemImage: summary.metrics.n == true ? "checkmark.seal.fill" : "info.circle")
                        .font(.footnote).foregroundStyle(summary.metrics.n == true ? AppTheme.positive : .secondary)
                }
            }
        }
    }

    @ViewBuilder private var overview: some View {
        if let profile = model.profile {
            ResearchCard(title: "公司资料", subtitle: "\(profile.source) · \(profile.updatedAt)") {
                if !profile.sector.isEmpty || !profile.industry.isEmpty {
                    Text([profile.sector, profile.industry].filter { !$0.isEmpty }.joined(separator: " · ")).font(.subheadline.weight(.medium))
                }
                Text(profile.description.isEmpty ? "暂无公司简介" : profile.description).font(.subheadline).foregroundStyle(.secondary)
                if let url = URL(string: profile.website), !profile.website.isEmpty { Link("访问公司网站", destination: url) }
            }
        }
        if let data = model.secondLayer {
            ResearchCard(title: "二层研究数据", subtitle: sourceLine(data)) {
                ForEach(data.keys.filter { !["symbol", "name", "source", "data_as_of"].contains($0) }.sorted(), id: \.self) { key in
                    HStack(alignment: .top) {
                        Text(displayKey(key)).foregroundStyle(.secondary)
                        Spacer()
                        Text(summaryText(data[key])).multilineTextAlignment(.trailing).lineLimit(3)
                    }.font(.subheadline)
                }
            }
        }
    }

    @ViewBuilder private var charts: some View {
        Picker("周期", selection: $model.period) {
            ForEach(KLinePeriod.allCases) { Text($0.title).tag($0) }
        }
        .pickerStyle(.segmented)
        .onChange(of: model.period) { _, value in Task { await model.changePeriod(value) } }

        if let data = model.kline { PriceChartCard(data: data) }
        if let data = model.rs { RSChartCard(data: data) }
        if let data = model.eps { EPSChartCard(data: data) }
        if let data = model.revenue { RevenueChartCard(data: data) }
        if let data = model.margin { MarginChartCard(data: data) }
    }

    private func sourceLine(_ data: [String: JSONValue]) -> String {
        [summaryText(data["source"]), summaryText(data["data_as_of"])].filter { !$0.isEmpty }.joined(separator: " · ")
    }
    private func displayKey(_ key: String) -> String { key.replacingOccurrences(of: "_", with: " ").capitalized }
    private func summaryText(_ value: JSONValue?) -> String {
        switch value {
        case let .string(value): value
        case let .number(value): value.formatted(.number.precision(.fractionLength(0...2)))
        case let .bool(value): value ? "是" : "否"
        case let .array(value): "\(value.count) 项"
        case let .object(value): "\(value.count) 项"
        default: ""
        }
    }
}

private struct SummaryChip: View {
    let label: String
    let value: Double
    let sufficient: Bool
    var body: some View {
        VStack(spacing: 3) {
            Text(label).font(.caption.bold()).foregroundStyle(.secondary)
            Text(sufficient ? value.formatted(.number.precision(.fractionLength(0...1))) : "—").font(.subheadline.monospacedDigit().weight(.semibold))
        }.frame(maxWidth: .infinity).padding(.vertical, 9).background(Color.secondary.opacity(0.09), in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct PriceChartCard: View {
    let data: KLineData
    var candles: [Candle] { Array(data.candles.suffix(60)) }
    var body: some View {
        ResearchCard(title: "价格", subtitle: "\(data.source) · \(data.dataAsOf)") {
            Chart(candles) { candle in
                RuleMark(x: .value("日期", candle.date), yStart: .value("最低", candle.low), yEnd: .value("最高", candle.high))
                    .foregroundStyle(candle.close >= candle.open ? AppTheme.positive : AppTheme.negative)
                RectangleMark(x: .value("日期", candle.date), yStart: .value("开盘", candle.open), yEnd: .value("收盘", candle.close), width: 4)
                    .foregroundStyle(candle.close >= candle.open ? AppTheme.positive : AppTheme.negative)
            }.chartXAxis(.hidden).frame(height: 220)
        }
    }
}

private struct RSChartCard: View {
    let data: RSData
    var body: some View {
        ResearchCard(title: "相对强度", subtitle: "\(data.source) · \(data.dataAsOf)") {
            Chart(Array(data.rsLine.suffix(100))) { point in
                LineMark(x: .value("日期", point.date), y: .value("RS", point.value)).foregroundStyle(AppTheme.accent).interpolationMethod(.catmullRom)
                RuleMark(y: .value("强势线", 80)).foregroundStyle(AppTheme.warning.opacity(0.7)).lineStyle(.init(dash: [4]))
            }.chartXAxis(.hidden).frame(height: 180)
        }
    }
}

private struct EPSChartCard: View {
    let data: EPSData
    var body: some View {
        ResearchCard(title: "季度 EPS", subtitle: "\(data.source) · \(data.dataAsOf)") {
            Chart(data.quarters) { item in
                BarMark(x: .value("季度", item.period), y: .value("EPS", item.eps)).foregroundStyle(item.eps >= 0 ? AppTheme.positive : AppTheme.negative)
            }.chartXAxis(.hidden).frame(height: 180)
        }
    }
}

private struct RevenueChartCard: View {
    let data: RevenueData
    var body: some View {
        ResearchCard(title: "年度营收", subtitle: "\(data.source) · \(data.dataAsOf)") {
            Chart(data.years) { item in BarMark(x: .value("年度", String(item.year)), y: .value("营收", item.revenue)).foregroundStyle(AppTheme.accent.gradient) }
                .chartXAxis(.hidden).frame(height: 180)
        }
    }
}

private struct MarginChartCard: View {
    let data: GrossMarginData
    var body: some View {
        ResearchCard(title: "季度毛利率", subtitle: "\(data.source) · \(data.dataAsOf)") {
            Chart(data.quarters) { item in LineMark(x: .value("季度", item.period), y: .value("毛利率", item.grossMargin)).foregroundStyle(AppTheme.warning).symbol(.circle) }
                .chartXAxis(.hidden).frame(height: 180)
        }
    }
}

private struct AIAnalysisView: View {
    let symbol: String
    @ObservedObject var research: AnalysisViewModel
    @ObservedObject var controller: AIStreamController
    @State private var showsReplaceConfirmation = false
    @State private var shareURL: URL?

    var body: some View {
        ResearchCard(title: "多智能体研究", subtitle: "实时进度与可审计报告") {
            if controller.isLoadingConfig { ProgressView("正在读取 AI 配置") }
            else if controller.configResponse == nil {
                Button("读取配置") { Task { await controller.loadConfig() } }.buttonStyle(.borderedProminent)
            } else {
                AIConfigForm(controller: controller)
                HStack {
                    if controller.status == .streaming {
                        Button("暂停", systemImage: "pause.circle") { controller.pause() }.buttonStyle(.bordered)
                    } else if [.paused, .error].contains(controller.status), controller.runID != nil {
                        Button("继续分析", systemImage: "play.circle") { controller.resume(research: research) }.buttonStyle(.borderedProminent)
                    } else {
                        Button(controller.report.isEmpty ? "开始分析" : "重新分析", systemImage: "sparkles") {
                            if controller.report.isEmpty { controller.start(research: research) }
                            else { showsReplaceConfirmation = true }
                        }.buttonStyle(.borderedProminent).disabled(controller.config == nil)
                    }
                    Spacer()
                    AIStatusBadge(status: controller.status)
                }
            }
            if let error = controller.error { Text(error).font(.footnote).foregroundStyle(AppTheme.negative) }
        }
        .task { await controller.loadConfig() }
        if !controller.progress.isEmpty {
            ResearchCard(title: "分析进度") {
                ForEach(controller.progress) { item in
                    HStack(alignment: .top) {
                        Image(systemName: item.status == "done" ? "checkmark.circle.fill" : item.status == "error" ? "xmark.circle.fill" : "circle.dotted")
                            .foregroundStyle(item.status == "done" ? AppTheme.positive : item.status == "error" ? AppTheme.negative : AppTheme.accent)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(item.agent).font(.subheadline.weight(.semibold))
                            if !item.message.isEmpty { Text(item.message).font(.caption).foregroundStyle(.secondary) }
                        }
                    }
                }
            }
        }
        if let decision = controller.decision {
            ResearchCard(title: "最终建议", subtitle: "\(decision.action) · \(decision.confidence)") {
                Text(decision.summary)
            }
        }
        if !controller.report.isEmpty || !controller.narration.isEmpty {
            ResearchCard(title: controller.report.isEmpty ? "实时解说" : "完整报告") {
                MarkdownText(controller.report.isEmpty ? controller.narration : controller.report)
                if controller.status != .streaming {
                    HStack {
                        Button("写回 N 标签", systemImage: "n.circle") { Task { await controller.writeNLabel(research: research) } }.buttonStyle(.bordered)
                        Spacer()
                        Button("准备分享", systemImage: "doc.badge.arrow.up") { shareURL = try? controller.reportFile() }.buttonStyle(.bordered)
                        if let shareURL { ShareLink(item: shareURL) { Image(systemName: "square.and.arrow.up") }.accessibilityLabel("分享 Markdown 报告") }
                    }
                }
            }
        }
        EmptyView()
            .confirmationDialog("确认重新分析？", isPresented: $showsReplaceConfirmation, titleVisibility: .visible) {
                Button("重新分析", role: .destructive) { controller.start(research: research, replacing: true) }
            } message: { Text("当前进度、报告和最终建议会被新结果覆盖。") }
    }
}

private struct AIConfigForm: View {
    @ObservedObject var controller: AIStreamController
    var body: some View {
        if let response = controller.configResponse, let config = Binding($controller.config) {
            DisclosureGroup("分析配置") {
                VStack(spacing: 12) {
                    Picker("供应商", selection: config.llmProvider) {
                        ForEach(response.providers) { Text($0.name).tag($0.id) }
                    }
                    .onChange(of: config.wrappedValue.llmProvider) { _, value in controller.selectProvider(value) }
                    if let provider = response.providers.first(where: { $0.id == config.wrappedValue.llmProvider }) {
                        Picker("深度模型", selection: config.deepThinkLlm) { ForEach(provider.models.deep, id: \.self) { Text($0).tag($0) } }
                        Picker("快速模型", selection: config.quickThinkLlm) { ForEach(provider.models.quick, id: \.self) { Text($0).tag($0) } }
                        if provider.customModels ?? false { TextField("自定义网关", text: config.backendUrl).textInputAutocapitalization(.never).autocorrectionDisabled().textFieldStyle(.roundedBorder) }
                    }
                    Picker("研究辩论轮数", selection: config.maxDebateRounds) { ForEach(response.debateRounds, id: \.self) { Text("\($0)").tag($0) } }
                    Picker("风险辩论轮数", selection: config.maxRiskRounds) { ForEach(response.riskRounds, id: \.self) { Text("\($0)").tag($0) } }
                    Toggle("启用在线工具", isOn: config.onlineTools)
                    VStack(alignment: .leading, spacing: 8) {
                        Text("分析师").font(.caption).foregroundStyle(.secondary)
                        ForEach(response.analysts) { analyst in
                            Toggle(isOn: Binding(get: { config.wrappedValue.analysts.contains(analyst.id) }, set: { enabled in
                                if enabled { config.wrappedValue.analysts.append(analyst.id) }
                                else { config.wrappedValue.analysts.removeAll { $0 == analyst.id } }
                            })) { VStack(alignment: .leading) { Text(analyst.name); Text(analyst.description).font(.caption).foregroundStyle(.secondary) } }
                        }
                    }
                }.padding(.top, 10)
            }
        }
    }
}

private struct AIStatusBadge: View {
    let status: AIStatus
    var body: some View {
        let label = status == .streaming ? "生成中" : status == .paused ? "已暂停" : status == .done ? "已完成" : status == .error ? "已中断" : "待开始"
        Text(label).font(.caption.weight(.semibold)).padding(.horizontal, 9).padding(.vertical, 5)
            .background((status == .done ? AppTheme.positive : status == .error ? AppTheme.negative : status == .streaming ? AppTheme.accent : Color.secondary).opacity(0.14), in: Capsule())
    }
}

private struct MarkdownText: View {
    let value: String
    init(_ value: String) { self.value = value }
    var body: some View {
        if let attributed = try? AttributedString(markdown: value) { Text(attributed).textSelection(.enabled) }
        else { Text(value).textSelection(.enabled) }
    }
}
