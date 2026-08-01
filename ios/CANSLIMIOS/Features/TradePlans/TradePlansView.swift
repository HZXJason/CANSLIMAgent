import SwiftUI

@MainActor
final class TradePlansViewModel: ObservableObject {
    @Published var plans: [TradePlan] = []
    @Published var query = ""
    @Published var status: TradePlanStatus?
    @Published var isLoading = false
    @Published var error: String?

    var filtered: [TradePlan] {
        plans.filter { plan in
            (query.isEmpty || plan.ticker.localizedCaseInsensitiveContains(query)) && (status == nil || plan.status == status)
        }
    }

    func load() async {
        isLoading = true
        error = nil
        do { plans = try await APIClient.shared.request("/trade-plans") }
        catch { self.error = error.localizedDescription }
        isLoading = false
    }

    func save(id: String?, input: TradePlanInput) async throws {
        let _: TradePlan = try await APIClient.shared.request(id.map { "/trade-plans/\($0)" } ?? "/trade-plans", method: id == nil ? "POST" : "PATCH", body: input)
        await load()
    }

    func execute(id: String, input: ExecuteTradePlanInput) async throws {
        let _: ExecuteTradePlanResponse = try await APIClient.shared.request("/trade-plans/\(id)/execute", method: "POST", body: input)
        await load()
    }
}

struct TradePlansView: View {
    @StateObject private var model = TradePlansViewModel()
    @State private var editor: TradePlan?
    @State private var showsNew = false

    var body: some View {
        NavigationStack {
            Group {
                if model.isLoading && model.plans.isEmpty { LoadingStateView(label: "正在读取交易计划") }
                else if let error = model.error, model.plans.isEmpty { ErrorStateView(message: error) { Task { await model.load() } } }
                else {
                    List(model.filtered) { plan in
                        Button { editor = plan } label: { TradePlanRow(plan: plan) }.buttonStyle(.plain)
                    }
                    .listStyle(.plain)
                    .refreshable { await model.load() }
                    .overlay { if model.filtered.isEmpty { ContentUnavailableView("暂无交易计划", systemImage: "checklist", description: Text("建立入场条件、风险边界和执行记录。")) } }
                }
            }
            .navigationTitle("计划")
            .searchable(text: $model.query, prompt: "股票代码")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Menu {
                        Button("全部状态") { model.status = nil }
                        ForEach(TradePlanStatus.allCases) { item in Button(item.title) { model.status = item } }
                    } label: { Label(model.status?.title ?? "全部", systemImage: "line.3.horizontal.decrease") }
                }
                ToolbarItem(placement: .primaryAction) { Button { showsNew = true } label: { Label("新建计划", systemImage: "plus") } }
            }
            .task { await model.load() }
            .sheet(isPresented: $showsNew) { TradePlanEditor(model: model) }
            .sheet(item: $editor) { TradePlanEditor(model: model, plan: $0) }
        }
    }
}

private struct TradePlanRow: View {
    let plan: TradePlan
    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            HStack {
                Text(plan.ticker).font(.headline.monospacedDigit())
                Text(plan.direction.title).font(.caption.weight(.semibold)).foregroundStyle(plan.direction == .buy ? AppTheme.positive : plan.direction == .sell ? AppTheme.negative : .secondary)
                Spacer()
                Text(plan.status.title).font(.caption).padding(.horizontal, 8).padding(.vertical, 4).background(Color.secondary.opacity(0.12), in: Capsule())
                Image(systemName: "chevron.right").font(.caption).foregroundStyle(.tertiary)
            }
            if let trigger = plan.entryTrigger, !trigger.isEmpty { Text(trigger).font(.subheadline).foregroundStyle(.secondary).lineLimit(2) }
            HStack {
                Text("计划 \(plan.planDate)")
                Spacer()
                if let stop = plan.stopLoss { Text("止损 \(stop.formatted())").foregroundStyle(AppTheme.negative) }
            }.font(.caption).foregroundStyle(.secondary)
        }.padding(.vertical, 6)
    }
}

struct TradePlanEditor: View {
    @ObservedObject var model: TradePlansViewModel
    var plan: TradePlan?
    @Environment(\.dismiss) private var dismiss
    @State private var ticker: String
    @State private var direction: TradePlanDirection
    @State private var status: TradePlanStatus
    @State private var planDate = Date()
    @State private var hasExpiry: Bool
    @State private var validUntil = Date()
    @State private var trigger: String
    @State private var entryPrice: String
    @State private var stopLoss: String
    @State private var takeProfit: String
    @State private var positionSize: String
    @State private var rationale: String
    @State private var aiSummary: String
    @State private var notes: String
    @State private var isSaving = false
    @State private var showsExecute = false
    @State private var error: String?

    init(
        model: TradePlansViewModel,
        plan: TradePlan? = nil,
        defaultTicker: String = "",
        defaultAISummary: String = ""
    ) {
        self.model = model; self.plan = plan
        _ticker = State(initialValue: plan?.ticker ?? defaultTicker)
        _direction = State(initialValue: plan?.direction ?? .watch)
        _status = State(initialValue: plan?.status ?? .draft)
        _hasExpiry = State(initialValue: plan?.validUntil != nil)
        _trigger = State(initialValue: plan?.entryTrigger ?? "")
        _entryPrice = State(initialValue: plan?.entryPrice.map { String($0) } ?? "")
        _stopLoss = State(initialValue: plan?.stopLoss.map { String($0) } ?? "")
        _takeProfit = State(initialValue: plan?.takeProfit.map { String($0) } ?? "")
        _positionSize = State(initialValue: plan?.positionSize.map { String($0) } ?? "")
        _rationale = State(initialValue: plan?.rationale ?? "")
        _aiSummary = State(initialValue: plan?.aiSummary ?? defaultAISummary)
        _notes = State(initialValue: plan?.notes ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("标的与状态") {
                    TextField("股票代码", text: $ticker).textInputAutocapitalization(.characters).autocorrectionDisabled()
                    Picker("方向", selection: $direction) { ForEach(TradePlanDirection.allCases) { Text($0.title).tag($0) } }
                    Picker("状态", selection: $status) { ForEach(TradePlanStatus.allCases.filter { $0 != .executed }) { Text($0.title).tag($0) } }
                    DatePicker("计划日期", selection: $planDate, displayedComponents: .date)
                    Toggle("设置有效期", isOn: $hasExpiry)
                    if hasExpiry { DatePicker("有效期至", selection: $validUntil, displayedComponents: .date) }
                }
                Section("执行条件") {
                    TextField("入场触发条件", text: $trigger, axis: .vertical)
                    numberField("计划入场价", text: $entryPrice)
                    numberField("止损价", text: $stopLoss)
                    numberField("止盈价", text: $takeProfit)
                    numberField("计划仓位", text: $positionSize)
                }
                Section("研究依据") {
                    TextField("计划理由", text: $rationale, axis: .vertical).lineLimit(3...8)
                    TextField("AI 摘要", text: $aiSummary, axis: .vertical).lineLimit(3...8)
                    TextField("备注", text: $notes, axis: .vertical).lineLimit(2...6)
                }
                if let plan, ![.executed, .cancelled, .expired].contains(plan.status) {
                    Section { Button("记录执行", systemImage: "checkmark.circle.fill") { showsExecute = true }.foregroundStyle(AppTheme.positive) }
                }
                if let error { Section { Text(error).foregroundStyle(AppTheme.negative) } }
            }
            .navigationTitle(plan == nil ? "新建计划" : ticker)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("保存") { Task { await save() } }.disabled(isSaving || ticker.trimmingCharacters(in: .whitespaces).isEmpty) }
            }
            .sheet(isPresented: $showsExecute) { if let plan { ExecutePlanSheet(model: model, plan: plan) { dismiss() } } }
        }
    }

    private func numberField(_ title: String, text: Binding<String>) -> some View {
        TextField(title, text: text).keyboardType(.decimalPad)
    }
    private func save() async {
        isSaving = true; defer { isSaving = false }
        let input = TradePlanInput(
            ticker: ticker.trimmingCharacters(in: .whitespacesAndNewlines).uppercased(), direction: direction, status: status,
            planDate: planDate.apiDate, validUntil: hasExpiry ? validUntil.apiDate : nil, entryTrigger: trigger.nilIfBlank,
            entryPrice: Double(entryPrice), stopLoss: Double(stopLoss), takeProfit: Double(takeProfit), positionSize: Double(positionSize),
            rationale: rationale.nilIfBlank, aiSummary: aiSummary.nilIfBlank, notes: notes.nilIfBlank)
        do { try await model.save(id: plan?.id, input: input); dismiss() }
        catch { self.error = error.localizedDescription }
    }
}

private struct ExecutePlanSheet: View {
    @ObservedObject var model: TradePlansViewModel
    let plan: TradePlan
    let completed: () -> Void
    @Environment(\.dismiss) private var dismiss
    @State private var date = Date()
    @State private var quantity = ""
    @State private var price: String
    @State private var fee = ""
    @State private var market: TradeMarket = SettingsStore.shared.assetMarket == .us ? .us : .chinaA
    @State private var error: String?

    init(model: TradePlansViewModel, plan: TradePlan, completed: @escaping () -> Void) {
        self.model = model; self.plan = plan; self.completed = completed
        _price = State(initialValue: plan.entryPrice.map { String($0) } ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                DatePicker("执行时间", selection: $date)
                TextField("数量", text: $quantity).keyboardType(.decimalPad)
                TextField("成交价", text: $price).keyboardType(.decimalPad)
                TextField("费用", text: $fee).keyboardType(.decimalPad)
                Picker("市场", selection: $market) { ForEach(TradeMarket.allCases) { Text($0.title).tag($0) } }
                if let error { Text(error).foregroundStyle(AppTheme.negative) }
            }
            .navigationTitle("执行 \(plan.ticker)").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("确认执行") { Task { await execute() } }.disabled(Double(quantity) == nil || Double(price) == nil)
                }
            }
        }
    }

    private func execute() async {
        guard let quantity = Double(quantity), let price = Double(price) else { return }
        let input = ExecuteTradePlanInput(datetime: date.apiDateTime, quantity: quantity, price: price, market: market,
                                          name: nil, fee: Double(fee), currency: nil, orderId: nil, notes: plan.notes)
        do { try await model.execute(id: plan.id, input: input); dismiss(); completed() }
        catch { self.error = error.localizedDescription }
    }
}

extension Date {
    var apiDate: String { formatted(.iso8601.year().month().day().dateSeparator(.dash)) }
    var apiDateTime: String { ISO8601DateFormatter().string(from: self) }
}

extension String {
    var nilIfBlank: String? { trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? nil : self }
}
