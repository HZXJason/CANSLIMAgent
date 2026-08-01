import SwiftUI

@MainActor
final class JournalViewModel: ObservableObject {
    @Published var entries: [JournalEntry] = []
    @Published var reflection: StoredJournalReflection?
    @Published var isLoading = false
    @Published var error: String?

    func load() async {
        isLoading = true; error = nil
        do {
            async let entries: [JournalEntry] = APIClient.shared.request("/journal")
            async let reflection: StoredJournalReflection? = APIClient.shared.request("/journal/reflections/latest")
            self.entries = try await entries
            self.reflection = try? await reflection
        } catch { self.error = error.localizedDescription }
        isLoading = false
    }
    func create(_ input: JournalInput) async throws {
        let _: JournalEntry = try await APIClient.shared.request("/journal", method: "POST", body: input)
        await load()
    }
    func patch(id: String, notes: String?, reflection: String?) async throws {
        let _: JournalEntry = try await APIClient.shared.request("/journal/\(id)", method: "PATCH", body: JournalPatch(notes: notes, reflection: reflection))
        await load()
    }
    func delete(id: String) async throws {
        let _: DeletedResponse = try await APIClient.shared.request("/journal/\(id)", method: "DELETE")
        await load()
    }
    func generateReflection() async throws {
        reflection = try await APIClient.shared.request("/journal/reflections", method: "POST")
    }
    func exportFile() async throws -> URL {
        let (data, _) = try await APIClient.shared.raw("/journal/export")
        let url = FileManager.default.temporaryDirectory.appendingPathComponent("canslim-journal-\(Date().apiDate).json")
        try data.write(to: url, options: .atomic)
        return url
    }
}

struct JournalView: View {
    @StateObject private var model = JournalViewModel()
    @State private var showsNew = false
    @State private var selected: JournalEntry?
    @State private var exportURL: URL?
    @State private var isReflecting = false

    var body: some View {
        NavigationStack {
            Group {
                if model.isLoading && model.entries.isEmpty { LoadingStateView(label: "正在读取交易日志") }
                else if let error = model.error, model.entries.isEmpty { ErrorStateView(message: error) { Task { await model.load() } } }
                else {
                    List {
                        if let reflection = model.reflection { ReflectionCard(reflection: reflection) }
                        ForEach(model.entries) { entry in
                            Button { selected = entry } label: { JournalRow(entry: entry) }.buttonStyle(.plain)
                        }
                    }
                    .listStyle(.plain).refreshable { await model.load() }
                    .overlay { if model.entries.isEmpty && model.reflection == nil { ContentUnavailableView("暂无交易日志", systemImage: "book.pages", description: Text("手动记录或从只读券商连接导入成交。")) } }
                }
            }
            .navigationTitle("日志")
            .toolbar {
                ToolbarItemGroup(placement: .topBarTrailing) {
                    Menu {
                        Button("生成行为复盘", systemImage: "sparkles") { Task { await generateReflection() } }
                        if let exportURL { ShareLink(item: exportURL) { Label("分享导出文件", systemImage: "square.and.arrow.up") } }
                        Button("准备导出", systemImage: "arrow.down.doc") { Task { exportURL = try? await model.exportFile() } }
                    } label: { if isReflecting { ProgressView() } else { Label("更多", systemImage: "ellipsis.circle") } }
                    Button { showsNew = true } label: { Label("新建日志", systemImage: "plus") }
                }
            }
            .task { await model.load() }
            .sheet(isPresented: $showsNew) { JournalEditor(model: model) }
            .sheet(item: $selected) { JournalEditor(model: model, entry: $0) }
        }
    }

    private func generateReflection() async {
        isReflecting = true; defer { isReflecting = false }
        do { try await model.generateReflection() } catch { model.error = error.localizedDescription }
    }
}

private struct JournalRow: View {
    let entry: JournalEntry
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(entry.symbol).font(.headline.monospacedDigit())
                Text(entry.side.title).font(.caption.bold()).foregroundStyle(entry.side == .buy ? AppTheme.positive : AppTheme.negative)
                Spacer()
                Text(entry.market.title).font(.caption).foregroundStyle(.secondary)
                Image(systemName: "chevron.right").font(.caption).foregroundStyle(.tertiary)
            }
            HStack {
                Text("\(entry.quantity.formatted()) × \(entry.price.formatted())")
                Spacer()
                Text(entry.amount.formatted(.currency(code: entry.currency.isEmpty ? "USD" : entry.currency)))
            }.font(.subheadline.monospacedDigit())
            Text(entry.datetime).font(.caption).foregroundStyle(.secondary)
        }.padding(.vertical, 6)
    }
}

private struct ReflectionCard: View {
    let reflection: StoredJournalReflection
    var body: some View {
        ResearchCard(title: "最新行为复盘", subtitle: "\(reflection.engine) · \(reflection.recordCount) 条记录") {
            if case let .string(text)? = reflection.payload["reflection_text"] { Text(text).font(.subheadline).lineLimit(8) }
            if case let .array(insights)? = reflection.payload["insights"] {
                ForEach(Array(insights.enumerated()), id: \.offset) { _, value in
                    if case let .string(text) = value { Label(text, systemImage: "lightbulb").font(.footnote) }
                }
            }
        }.listRowInsets(.init()).listRowSeparator(.hidden).padding(.vertical, 6)
    }
}

private struct JournalEditor: View {
    @ObservedObject var model: JournalViewModel
    var entry: JournalEntry?
    @Environment(\.dismiss) private var dismiss
    @State private var date = Date()
    @State private var symbol: String
    @State private var name: String
    @State private var side: TradeSide
    @State private var quantity: String
    @State private var price: String
    @State private var fee: String
    @State private var market: TradeMarket
    @State private var currency: String
    @State private var orderID: String
    @State private var notes: String
    @State private var reflection: String
    @State private var error: String?
    @State private var confirmsDelete = false

    init(model: JournalViewModel, entry: JournalEntry? = nil) {
        self.model = model; self.entry = entry
        _symbol = State(initialValue: entry?.symbol ?? "")
        _name = State(initialValue: entry?.name ?? "")
        _side = State(initialValue: entry?.side ?? .buy)
        _quantity = State(initialValue: entry.map { String($0.quantity) } ?? "")
        _price = State(initialValue: entry.map { String($0.price) } ?? "")
        _fee = State(initialValue: entry.map { String($0.fee) } ?? "")
        _market = State(initialValue: entry?.market ?? (SettingsStore.shared.assetMarket == .us ? .us : .chinaA))
        _currency = State(initialValue: entry?.currency ?? (SettingsStore.shared.assetMarket == .us ? "USD" : "CNY"))
        _orderID = State(initialValue: entry?.orderId ?? "")
        _notes = State(initialValue: entry?.notes ?? "")
        _reflection = State(initialValue: entry?.reflection ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                if entry == nil {
                    Section("成交") {
                        DatePicker("成交时间", selection: $date)
                        TextField("股票代码", text: $symbol).textInputAutocapitalization(.characters).autocorrectionDisabled()
                        TextField("名称（可选）", text: $name)
                        Picker("方向", selection: $side) { ForEach(TradeSide.allCases) { Text($0.title).tag($0) } }
                        TextField("数量", text: $quantity).keyboardType(.decimalPad)
                        TextField("价格", text: $price).keyboardType(.decimalPad)
                        TextField("费用", text: $fee).keyboardType(.decimalPad)
                        Picker("市场", selection: $market) { ForEach(TradeMarket.allCases) { Text($0.title).tag($0) } }
                        TextField("币种", text: $currency).textInputAutocapitalization(.characters)
                        TextField("订单号（可选）", text: $orderID)
                    }
                } else {
                    Section("成交") {
                        LabeledContent("代码", value: symbol)
                        LabeledContent("方向", value: side.title)
                        LabeledContent("数量 × 价格", value: "\(quantity) × \(price)")
                    }
                }
                Section("复盘") {
                    TextField("交易备注", text: $notes, axis: .vertical).lineLimit(3...8)
                    TextField("事后复盘", text: $reflection, axis: .vertical).lineLimit(4...12)
                }
                if entry != nil { Section { Button("删除日志", role: .destructive) { confirmsDelete = true } } }
                if let error { Section { Text(error).foregroundStyle(AppTheme.negative) } }
            }
            .navigationTitle(entry == nil ? "记录成交" : symbol).navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("保存") { Task { await save() } }.disabled(entry == nil && (symbol.isEmpty || Double(quantity) == nil || Double(price) == nil)) }
            }
            .confirmationDialog("确认删除这条交易日志？", isPresented: $confirmsDelete, titleVisibility: .visible) {
                Button("删除", role: .destructive) { Task { await delete() } }
            } message: { Text("删除后无法在应用内恢复。") }
        }
    }

    private func save() async {
        do {
            if let entry { try await model.patch(id: entry.id, notes: notes.nilIfBlank, reflection: reflection.nilIfBlank) }
            else {
                guard let quantity = Double(quantity), let price = Double(price) else { return }
                let input = JournalInput(datetime: date.apiDateTime, symbol: symbol.uppercased(), name: name.nilIfBlank, side: side,
                                         quantity: quantity, price: price, amount: nil, fee: Double(fee), market: market,
                                         currency: currency.nilIfBlank, orderId: orderID.nilIfBlank, notes: notes.nilIfBlank)
                try await model.create(input)
            }
            dismiss()
        } catch { self.error = error.localizedDescription }
    }
    private func delete() async {
        guard let entry else { return }
        do { try await model.delete(id: entry.id); dismiss() } catch { self.error = error.localizedDescription }
    }
}
