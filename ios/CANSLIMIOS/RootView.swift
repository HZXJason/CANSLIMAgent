import SwiftUI

enum AppTab: String, Hashable { case desk, screener, analysis, plans, journal, settings }

struct RootView: View {
    @State private var tab: AppTab
    @State private var analysisSymbol: String
    @State private var analysisSection: String

    init() {
        let environment = ProcessInfo.processInfo.environment
        let screen = environment["CANSLIM_SCREEN"] ?? "desk"
        let initialTab: AppTab = screen.hasPrefix("analysis") ? .analysis : AppTab(rawValue: screen) ?? .desk
        _tab = State(initialValue: initialTab)
        _analysisSymbol = State(initialValue: environment["CANSLIM_SYMBOL"] ?? (screen.hasPrefix("analysis") ? "AAPL" : ""))
        _analysisSection = State(initialValue: environment["CANSLIM_SECTION"] ?? (screen == "analysis-charts" ? "charts" : screen == "analysis-ai" ? "ai" : "overview"))
    }

    var body: some View {
        TabView(selection: $tab) {
            DeskCompanionView { symbol in
                analysisSymbol = symbol
                tab = .analysis
            }
            .tabItem { Label("作战室", systemImage: "shield.checkered") }
            .tag(AppTab.desk)

            ScreenerView { symbol in
                analysisSymbol = symbol
                tab = .analysis
            }
            .tabItem { Label("选股", systemImage: "line.3.horizontal.decrease.circle") }
            .tag(AppTab.screener)

            AnalysisHomeView(initialSymbol: $analysisSymbol, initialSection: $analysisSection)
                .tabItem { Label("分析", systemImage: "chart.xyaxis.line") }
                .tag(AppTab.analysis)

            TradePlansView()
                .tabItem { Label("计划", systemImage: "checklist") }
                .tag(AppTab.plans)

            JournalView()
                .tabItem { Label("日志", systemImage: "book.pages") }
                .tag(AppTab.journal)

            SettingsView()
                .tabItem { Label("设置", systemImage: "gearshape") }
                .tag(AppTab.settings)
        }
        .onOpenURL(perform: route)
    }

    private func route(_ url: URL) {
        guard url.scheme == "canslim" else { return }
        switch url.host?.lowercased() {
        case "desk": tab = .desk
        case "screener": tab = .screener
        case "analysis":
            let symbol = url.pathComponents.dropFirst().first?.removingPercentEncoding ?? ""
            analysisSection = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first(where: { $0.name == "section" })?.value ?? "overview"
            if !symbol.isEmpty { analysisSymbol = symbol.uppercased() }
            tab = .analysis
        case "plans": tab = .plans
        case "journal": tab = .journal
        case "settings": tab = .settings
        default: break
        }
    }
}

struct DeskCompanionView: View {
    let onOpenSymbol: (String) -> Void
    @State private var market = "US"
    @State private var desk: DeskPayload?
    @State private var isLoading = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            List {
                Picker("市场", selection: $market) {
                    Text("美股 US").tag("US")
                    Text("A股 CN").tag("CN")
                }
                .pickerStyle(.segmented)
                .onChange(of: market) { _, _ in
                    Task { await load() }
                }

                if let errorMessage {
                    Text(errorMessage).foregroundStyle(.red).font(.footnote)
                }

                if let desk {
                    Section("CANSLIM M · 大盘环境闸门") {
                        HStack {
                            Text(desk.marketRegime.regimeLabel).font(.headline)
                            Spacer()
                            Text("仓位上限 \(desk.marketRegime.maxExposurePercent)%")
                                .font(.subheadline.bold())
                        }
                        Text("近25日派发日：\(desk.marketRegime.distributionDays) 天 · \(desk.marketRegime.guidance)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }

                    Section("组合风控 (1R & Heat)") {
                        HStack {
                            Text("在途风险敞口")
                            Spacer()
                            Text(String(format: "%.2f%% / %.1f%%", desk.summary.openPortfolioRiskPercent, desk.summary.maxPortfolioHeatPercent))
                                .monospacedDigit()
                        }
                        HStack {
                            Text("合规风险闭环率")
                            Spacer()
                            Text(String(format: "%.1f%%", desk.summary.disciplinedRLoopRate))
                                .monospacedDigit()
                        }
                    }

                    Section("在途持仓 (\(desk.openTrades.count))") {
                        if desk.openTrades.isEmpty {
                            Text("暂无在途持仓").foregroundStyle(.secondary)
                        } else {
                            ForEach(desk.openTrades) { trade in
                                Button {
                                    onOpenSymbol(trade.symbol)
                                } label: {
                                    HStack {
                                        VStack(alignment: .leading) {
                                            Text(trade.symbol).font(.headline)
                                            Text("均价 \(String(format: "%.2f", trade.avgEntryPrice)) · 止损 \(trade.currentStopLoss.map { String(format: "%.2f", $0) } ?? "未设")")
                                                .font(.caption).foregroundStyle(.secondary)
                                        }
                                        Spacer()
                                        Text("\(String(format: "%.0f", trade.openRiskAmount)) \(trade.currency)")
                                            .font(.caption.monospacedDigit())
                                    }
                                }
                            }
                        }
                    }

                    Section("待触发计划 (\(desk.waitingPlans.count))") {
                        if desk.waitingPlans.isEmpty {
                            Text("暂无待触发计划").foregroundStyle(.secondary)
                        } else {
                            ForEach(desk.waitingPlans) { plan in
                                Button {
                                    onOpenSymbol(plan.ticker)
                                } label: {
                                    HStack {
                                        Text(plan.ticker).font(.headline)
                                        Spacer()
                                        Text("入场 \(plan.entryPrice.map { String(format: "%.2f", $0) } ?? "-") / 止损 \(plan.stopLoss.map { String(format: "%.2f", $0) } ?? "-")")
                                            .font(.caption.monospacedDigit())
                                            .foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                } else if isLoading {
                    ProgressView("正在加载作战室...")
                }
            }
            .navigationTitle("AlphaLens 作战室")
            .refreshable { await load() }
            .task { await load() }
        }
    }

    private func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            desk = try await APIClient.shared.fetchDesk(market: market)
            errorMessage = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}
