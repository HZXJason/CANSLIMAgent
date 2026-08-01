import SwiftUI

@MainActor
final class ScreenerViewModel: ObservableObject {
    @Published var market: Market = SettingsStore.shared.assetMarket == .us ? .sp500 : .sseMain
    @Published var stocks: [Stock] = []
    @Published var query = ""
    @Published var selectedLabels: Set<String> = []
    @Published var thresholds = ThresholdConfig()
    @Published var presets: [ThresholdPreset] = []
    @Published var activePresetID: Int?
    @Published var isLoading = false
    @Published var isRefreshing = false
    @Published var isScanning = false
    @Published var scanPaused = false
    @Published var scanAnalyzed = 0
    @Published var scanTotal = 0
    @Published var scanStatus: String?
    @Published var error: String?
    private var scanTask: Task<Void, Never>?

    var availableMarkets: [Market] { Market.allCases.filter { $0.assetMarket == SettingsStore.shared.assetMarket } }
    var filteredStocks: [Stock] {
        stocks.filter { stock in
            let matchesQuery = query.isEmpty || stock.symbol.localizedCaseInsensitiveContains(query) || stock.name.localizedCaseInsensitiveContains(query)
            return matchesQuery && selectedLabels.allSatisfy { labelPasses($0, stock: stock) }
        }
    }

    func load() async {
        isLoading = true
        error = nil
        do {
            async let loadedStocks: [Stock] = APIClient.shared.request("/screener/results", query: [.init(name: "market", value: market.rawValue)])
            async let loadedPresets: [ThresholdPreset] = APIClient.shared.request("/presets")
            stocks = try await loadedStocks
            presets = (try? await loadedPresets) ?? presets
            if activePresetID == nil, let defaultPreset = presets.first(where: \.isDefault) { apply(defaultPreset) }
        } catch { self.error = error.localizedDescription }
        isLoading = false
    }

    func refresh() async {
        isRefreshing = true
        error = nil
        do {
            let response: RefreshJobResponse = try await APIClient.shared.request("/screener/refresh", method: "POST", body: ["market": market.rawValue])
            for _ in 0..<180 {
                try await Task.sleep(for: .seconds(1))
                let job: RefreshJob = try await APIClient.shared.request("/screener/refresh/\(response.jobId)")
                if job.status == "completed" { await load(); break }
                if job.status == "failed" { throw APIError.message(job.error ?? "市场数据刷新失败") }
            }
        } catch { self.error = error.localizedDescription }
        isRefreshing = false
    }

    func switchAssetMarket(_ assetMarket: AssetMarket) {
        SettingsStore.shared.assetMarket = assetMarket
        market = assetMarket == .us ? .sp500 : .sseMain
    }

    func apply(_ preset: ThresholdPreset) { activePresetID = preset.id; thresholds = preset.thresholds }

    func savePreset(name: String) async {
        do {
            let preset: ThresholdPreset = try await APIClient.shared.request("/presets", method: "POST", body: PresetInput(name: name, thresholds: thresholds))
            presets.append(preset); apply(preset)
        } catch { self.error = error.localizedDescription }
    }

    func deleteActivePreset() async {
        guard let id = activePresetID, let preset = presets.first(where: { $0.id == id }), !preset.isDefault else { return }
        do {
            let _: DeletedResponse = try await APIClient.shared.request("/presets/\(id)", method: "DELETE")
            presets.removeAll { $0.id == id }; activePresetID = nil
        } catch { self.error = error.localizedDescription }
    }

    func startScan() {
        scanTask?.cancel()
        stocks = []
        scanAnalyzed = 0; scanTotal = 0; scanStatus = "连接市场数据"; error = nil
        isScanning = true; scanPaused = false
        scanTask = Task { [weak self] in
            guard let self else { return }
            do {
                let (bytes, _) = try await APIClient.shared.stream("/screener/scan", query: [.init(name: "market", value: market.rawValue)], body: Optional<[String: String]>.none)
                var block: [String] = []
                let decoder = JSONDecoder(); decoder.keyDecodingStrategy = .convertFromSnakeCase
                for try await line in bytes.lines {
                    try Task.checkCancellation()
                    if line.isEmpty {
                        if let event = SSEParser.parse(block.joined(separator: "\n")), let wire = try? decoder.decode(ScanWireEvent.self, from: Data(event.data.utf8)) { handle(wire) }
                        block.removeAll(keepingCapacity: true)
                    } else { block.append(line) }
                }
                if isScanning { isScanning = false }
            } catch is CancellationError {
                isScanning = false
            } catch {
                self.error = error.localizedDescription; isScanning = false
            }
        }
    }

    func pauseScan() { scanTask?.cancel(); scanTask = nil; isScanning = false; scanPaused = true; scanStatus = "已暂停" }

    private func handle(_ event: ScanWireEvent) {
        switch event {
        case let .stock(stock):
            if let index = stocks.firstIndex(where: { $0.symbol == stock.symbol && $0.market == stock.market }) { stocks[index] = stock }
            else { stocks.append(stock) }
        case let .status(message): scanStatus = message.nilIfBlank
        case let .progress(analyzed, total): scanAnalyzed = analyzed; scanTotal = total; if total > 0 { scanStatus = nil }
        case let .complete(analyzed, total): scanAnalyzed = analyzed; scanTotal = total; isScanning = false; scanPaused = false; scanStatus = "扫描完成"
        case let .error(message): error = message; isScanning = false; scanPaused = false
        }
    }

    private func labelPasses(_ label: String, stock: Stock) -> Bool {
        switch label {
        case "C": stock.metrics.c.sufficient && stock.metrics.c.value >= thresholds.cMinGrowth
        case "A": stock.metrics.a.sufficient && stock.metrics.a.value.count >= thresholds.aMinYears && stock.metrics.a.value.prefix(thresholds.aMinYears).allSatisfy { $0 >= thresholds.aMinGrowth }
        case "S": stock.metrics.s.sufficient && stock.metrics.s.value >= thresholds.sVolumeRatio
        case "L": stock.metrics.l.sufficient && stock.metrics.l.value >= thresholds.lMinRs
        case "I": stock.metrics.i.sufficient && stock.metrics.i.value >= thresholds.iMinDelta
        case "N": stock.metrics.n == true
        default: true
        }
    }
}

struct ScreenerView: View {
    @StateObject private var model = ScreenerViewModel()
    @State private var showsFilters = false
    let openAnalysis: (String) -> Void

    var body: some View {
        NavigationStack {
            Group {
                if model.isLoading && model.stocks.isEmpty {
                    LoadingStateView(label: "正在读取选股结果")
                } else if let error = model.error, model.stocks.isEmpty {
                    ErrorStateView(message: error) { Task { await model.load() } }
                } else {
                    List(model.filteredStocks) { stock in
                        Button { openAnalysis(stock.symbol) } label: { StockRow(stock: stock) }
                            .buttonStyle(.plain)
                            .listRowSeparator(.hidden)
                            .listRowBackground(Color.clear)
                    }
                    .listStyle(.plain)
                    .refreshable { await model.load() }
                    .overlay {
                        if model.filteredStocks.isEmpty {
                            ContentUnavailableView("没有匹配结果", systemImage: "line.3.horizontal.decrease.circle", description: Text("调整代码、名称或 CANSLIM 标签后重试。"))
                        }
                    }
                }
            }
            .navigationTitle("选股")
            .searchable(text: $model.query, prompt: "代码或公司名称")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Menu {
                        Picker("资产市场", selection: Binding(
                            get: { SettingsStore.shared.assetMarket },
                            set: { model.switchAssetMarket($0); Task { await model.load() } }
                        )) {
                            ForEach(AssetMarket.allCases) { Text($0.title).tag($0) }
                        }
                        Divider()
                        Picker("股票池", selection: $model.market) {
                            ForEach(model.availableMarkets) { Text($0.title).tag($0) }
                        }
                    } label: { Label(model.market.title, systemImage: "globe.asia.australia") }
                }
                ToolbarItemGroup(placement: .topBarTrailing) {
                    Button { showsFilters = true } label: { Label("筛选", systemImage: "slider.horizontal.3") }
                    Button { Task { await model.refresh() } } label: {
                        if model.isRefreshing { ProgressView() } else { Label("刷新数据", systemImage: "arrow.clockwise") }
                    }
                    .disabled(model.isRefreshing)
                }
            }
            .safeAreaInset(edge: .top) {
                if model.isScanning || model.scanPaused || model.scanTotal > 0 {
                    ScanProgressBar(model: model)
                } else if let error = model.error, !model.stocks.isEmpty {
                    Text(error).font(.footnote).foregroundStyle(.white).padding(10).frame(maxWidth: .infinity).background(AppTheme.negative)
                }
            }
            .onChange(of: model.market) { _, _ in Task { await model.load() } }
            .task { await model.load() }
            .sheet(isPresented: $showsFilters) { FilterSheet(model: model) }
        }
    }
}

private struct StockRow: View {
    let stock: Stock

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(stock.symbol).font(.headline.monospacedDigit())
                    Text(stock.name).font(.subheadline).foregroundStyle(.secondary).lineLimit(1)
                }
                Spacer()
                if stock.aiAnalyzed { Image(systemName: "sparkles").foregroundStyle(AppTheme.accent).accessibilityLabel("已完成 AI 分析") }
                Image(systemName: "chevron.right").font(.caption.weight(.semibold)).foregroundStyle(.tertiary)
            }
            HStack(spacing: 7) {
                MetricChip(label: "C", pass: stock.metrics.c.sufficient && stock.metrics.c.value >= 25)
                MetricChip(label: "A", pass: stock.metrics.a.sufficient && stock.metrics.a.value.filter { $0 >= 20 }.count >= 3)
                MetricChip(label: "S", pass: stock.metrics.s.sufficient && stock.metrics.s.value >= 1.5)
                MetricChip(label: "L", pass: stock.metrics.l.sufficient && stock.metrics.l.value >= 80)
                MetricChip(label: "I", pass: stock.metrics.i.sufficient && stock.metrics.i.value > 0)
                MetricChip(label: "N", pass: stock.metrics.n == true)
                Spacer(minLength: 0)
            }
            Text("数据截至 \(stock.dataAsOf)").font(.caption2).foregroundStyle(.tertiary)
        }
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: AppTheme.cardCorner, style: .continuous))
        .contentShape(Rectangle())
    }
}

private struct MetricChip: View {
    let label: String
    let pass: Bool
    var body: some View {
        Text(label).font(.caption.bold()).frame(width: 28, height: 28)
            .foregroundStyle(pass ? Color.white : Color.secondary)
            .background(pass ? AppTheme.positive : Color.secondary.opacity(0.12), in: Circle())
            .accessibilityLabel("\(label) \(pass ? "通过" : "未通过")")
    }
}

private struct ScanProgressBar: View {
    @ObservedObject var model: ScreenerViewModel
    var body: some View {
        VStack(spacing: 7) {
            HStack {
                Text(model.scanStatus ?? "已分析 \(model.scanAnalyzed) / \(model.scanTotal)").font(.caption.weight(.medium)).lineLimit(1)
                Spacer()
                if model.isScanning { Button("暂停", systemImage: "pause.fill") { model.pauseScan() }.font(.caption) }
                else { Button(model.scanPaused ? "重新开始" : "开始扫描", systemImage: "play.fill") { model.startScan() }.font(.caption) }
            }
            ProgressView(value: Double(model.scanAnalyzed), total: Double(max(model.scanTotal, 1)))
        }.padding(.horizontal, 16).padding(.vertical, 10).background(.ultraThinMaterial)
    }
}

private struct FilterSheet: View {
    @ObservedObject var model: ScreenerViewModel
    @Environment(\.dismiss) private var dismiss
    @State private var presetName = ""
    @State private var confirmsDelete = false

    var body: some View {
        NavigationStack {
            Form {
                Section("方案") {
                    Picker("阈值方案", selection: Binding(get: { model.activePresetID }, set: { value in
                        if let value, let preset = model.presets.first(where: { $0.id == value }) { model.apply(preset) }
                        else { model.activePresetID = nil }
                    })) {
                        Text("自定义").tag(Int?.none)
                        ForEach(model.presets) { Text($0.name.replacingOccurrences(of: "奥尼尔", with: "欧奈尔")).tag(Optional($0.id)) }
                    }
                    HStack {
                        TextField("新方案名称", text: $presetName)
                        Button("保存") { Task { await model.savePreset(name: presetName); presetName = "" } }.disabled(presetName.nilIfBlank == nil)
                    }
                    if let id = model.activePresetID, model.presets.first(where: { $0.id == id })?.isDefault == false {
                        Button("删除当前方案", role: .destructive) { confirmsDelete = true }
                    }
                }
                Section("筛选阈值") {
                    ThresholdStepper(title: "C · 当季 EPS 增长", value: $model.thresholds.cMinGrowth, range: 1...200, suffix: "%")
                    ThresholdStepper(title: "A · 年度 EPS 增长", value: $model.thresholds.aMinGrowth, range: 1...200, suffix: "%")
                    Stepper("A · 连续 \(model.thresholds.aMinYears) 年", value: $model.thresholds.aMinYears, in: 1...5)
                    ThresholdStepper(title: "S · 成交量比例", value: $model.thresholds.sVolumeRatio, range: 100...500, step: 5, suffix: "%")
                    Stepper("S · 均量 \(model.thresholds.sMaPeriod) 日", value: $model.thresholds.sMaPeriod, in: 10...60)
                    ThresholdStepper(title: "L · RS 评级", value: $model.thresholds.lMinRs, range: 50...99)
                    ThresholdStepper(title: "I · 机构增持", value: $model.thresholds.iMinDelta, range: 0...100, suffix: "%")
                }
                Section("必须通过") {
                    ForEach(["C", "A", "S", "L", "I", "N"], id: \.self) { label in
                        Button {
                            if model.selectedLabels.contains(label) { model.selectedLabels.remove(label) } else { model.selectedLabels.insert(label) }
                        } label: {
                            HStack {
                                Text(label).font(.body.bold()).foregroundStyle(.primary)
                                Spacer()
                                if model.selectedLabels.contains(label) { Image(systemName: "checkmark").foregroundStyle(AppTheme.accent) }
                            }
                        }
                    }
                }
                Section {
                    Button("开始实时扫描", systemImage: "waveform.path.ecg") { model.startScan(); dismiss() }
                    Button("清除标签筛选", role: .destructive) { model.selectedLabels.removeAll() }
                }
            }
            .navigationTitle("CANSLIM 筛选")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("完成") { dismiss() } } }
            .confirmationDialog("删除当前阈值方案？", isPresented: $confirmsDelete, titleVisibility: .visible) {
                Button("删除", role: .destructive) { Task { await model.deleteActivePreset() } }
            }
        }
        .presentationDetents([.large])
    }
}

private struct ThresholdStepper: View {
    let title: String
    @Binding var value: Double
    let range: ClosedRange<Double>
    var step: Double = 1
    var suffix = ""
    var body: some View {
        Stepper(value: $value, in: range, step: step) {
            HStack { Text(title); Spacer(); Text(value.formatted(.number.precision(.fractionLength(0...1))) + suffix).foregroundStyle(.secondary).monospacedDigit() }
        }
    }
}
