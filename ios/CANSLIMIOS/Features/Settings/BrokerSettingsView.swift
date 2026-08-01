import SwiftUI

@MainActor
final class BrokerSettingsViewModel: ObservableObject {
    @Published var connections: [BrokerConnection] = []
    @Published var syncPreview: BrokerSyncResult?
    @Published var syncingConnection: BrokerConnection?
    @Published var isLoading = false
    @Published var error: String?
    @Published var message: String?

    func load() async {
        isLoading = true; error = nil
        do { connections = try await APIClient.shared.request("/brokers") }
        catch { self.error = error.localizedDescription }
        isLoading = false
    }

    func connect(_ broker: BrokerID, openURL: OpenURLAction) async {
        do {
            let result: BrokerConnectionStart = try await APIClient.shared.request("/brokers/connect/\(broker.rawValue)", method: "POST")
            if result.action == "redirect", let string = result.authorizationUrl, let url = URL(string: string) { openURL(url) }
            else { await load() }
        } catch { self.error = error.localizedDescription }
    }

    func disconnect(_ connection: BrokerConnection) async {
        do {
            let _: DeletedResponse = try await APIClient.shared.request("/brokers/\(connection.id)", method: "DELETE")
            await load()
        } catch { self.error = error.localizedDescription }
    }

    func sync(_ connection: BrokerConnection) async {
        syncingConnection = connection; error = nil
        do { syncPreview = try await APIClient.shared.request("/brokers/\(connection.id)/sync", method: "POST", body: BrokerSyncInput(startAt: nil, endAt: nil)) }
        catch { self.error = error.localizedDescription; syncingConnection = nil }
    }

    func importAll() async {
        guard let connection = syncingConnection, let preview = syncPreview else { return }
        do {
            let result: BrokerImportResponse = try await APIClient.shared.request("/brokers/\(connection.id)/import", method: "POST", body: BrokerImportInput(executions: preview.executions))
            message = "已导入 \(result.created) 条，跳过 \(result.skipped) 条"
            syncPreview = nil; syncingConnection = nil
            await load()
        } catch { self.error = error.localizedDescription }
    }
}
struct BrokerSettingsView: View {
    @Environment(\.openURL) private var openURL
    @StateObject private var model = BrokerSettingsViewModel()
    @State private var disconnectTarget: BrokerConnection?

    var body: some View {
        List {
            Section("支持的只读连接") {
                ForEach(BrokerID.allCases) { broker in
                    let connection = model.connections.first { $0.broker == broker }
                    VStack(alignment: .leading, spacing: 10) {
                        HStack {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(broker.title).font(.headline)
                                Text(connection?.message.nilIfBlank ?? connection?.connectionStatus ?? "未连接").font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Circle().fill(connection?.connected == true ? AppTheme.positive : Color.secondary.opacity(0.35)).frame(width: 9, height: 9)
                        }
                        if let connection, connection.connected {
                            HStack {
                                Button("同步预览") { Task { await model.sync(connection) } }.buttonStyle(.borderedProminent)
                                Button("断开", role: .destructive) { disconnectTarget = connection }.buttonStyle(.bordered)
                            }
                            if let date = connection.lastSyncedAt { Text("上次同步 \(date)").font(.caption2).foregroundStyle(.tertiary) }
                        } else {
                            Button(broker == .futu || broker == .ibkr ? "检测本地网关" : "连接账户") { Task { await model.connect(broker, openURL: openURL) } }.buttonStyle(.bordered)
                        }
                    }.padding(.vertical, 5)
                }
            }
            if let message = model.message { Section { Text(message).foregroundStyle(AppTheme.positive) } }
            if let error = model.error { Section { Text(error).foregroundStyle(AppTheme.negative) } }
        }
        .navigationTitle("券商连接").navigationBarTitleDisplayMode(.inline)
        .overlay { if model.isLoading && model.connections.isEmpty { LoadingStateView(label: "正在读取账户连接") } }
        .task { await model.load() }
        .refreshable { await model.load() }
        .sheet(isPresented: Binding(get: { model.syncPreview != nil }, set: { if !$0 { model.syncPreview = nil; model.syncingConnection = nil } })) {
            if let preview = model.syncPreview { BrokerSyncPreview(model: model, result: preview) }
        }
        .confirmationDialog("断开 \(disconnectTarget?.label ?? "券商连接")？", isPresented: Binding(get: { disconnectTarget != nil }, set: { if !$0 { disconnectTarget = nil } }), titleVisibility: .visible) {
            Button("断开连接", role: .destructive) { if let target = disconnectTarget { Task { await model.disconnect(target); disconnectTarget = nil } } }
        } message: { Text("只会移除本地保存的授权，不会修改券商账户。") }
    }
}

private struct BrokerSyncPreview: View {
    @ObservedObject var model: BrokerSettingsViewModel
    let result: BrokerSyncResult
    @Environment(\.dismiss) private var dismiss
    var body: some View {
        NavigationStack {
            List {
                if !result.warnings.isEmpty { Section("警告") { ForEach(result.warnings, id: \.self) { Label($0, systemImage: "exclamationmark.triangle").foregroundStyle(AppTheme.warning) } } }
                Section("持仓（\(result.positions.count)）") {
                    ForEach(result.positions) { position in LabeledContent(position.symbol, value: position.quantity.formatted()) }
                }
                Section("待导入成交（\(result.executions.count)）") {
                    ForEach(result.executions) { execution in
                        VStack(alignment: .leading) {
                            HStack { Text(execution.symbol).font(.headline); Spacer(); Text(execution.side.uppercased()) }
                            Text("\(execution.quantity.formatted()) × \(execution.price.formatted()) · \(execution.executedAt)").font(.caption).foregroundStyle(.secondary)
                        }
                    }
                }
            }
            .navigationTitle("同步预览").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("导入全部") { Task { await model.importAll(); dismiss() } }.disabled(result.executions.isEmpty) }
            }
        }
    }
}
