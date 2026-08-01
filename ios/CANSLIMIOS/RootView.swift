import SwiftUI

enum AppTab: String, Hashable { case screener, analysis, plans, journal, settings }

struct RootView: View {
    @State private var tab: AppTab
    @State private var analysisSymbol: String
    @State private var analysisSection: String

    init() {
        let environment = ProcessInfo.processInfo.environment
        let screen = environment["CANSLIM_SCREEN"] ?? "screener"
        let initialTab: AppTab = screen.hasPrefix("analysis") ? .analysis : AppTab(rawValue: screen) ?? .screener
        _tab = State(initialValue: initialTab)
        _analysisSymbol = State(initialValue: environment["CANSLIM_SYMBOL"] ?? (screen.hasPrefix("analysis") ? "AAPL" : ""))
        _analysisSection = State(initialValue: environment["CANSLIM_SECTION"] ?? (screen == "analysis-charts" ? "charts" : screen == "analysis-ai" ? "ai" : "overview"))
    }

    var body: some View {
        TabView(selection: $tab) {
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
