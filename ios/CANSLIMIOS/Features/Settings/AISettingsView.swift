import SwiftUI

@MainActor
final class AISettingsViewModel: ObservableObject {
    @Published var response: AIAnalysisConfigResponse?
    @Published var statuses: [String: LLMCredentialStatus] = [:]
    @Published var discoveredModels: [String: [String]] = [:]
    @Published var isLoading = false
    @Published var error: String?

    func load() async {
        isLoading = true; error = nil
        do {
            let response: AIAnalysisConfigResponse = try await APIClient.shared.request("/analysis/ai/config")
            self.response = response
            for provider in response.providers { await loadStatus(provider: provider.id, backendURL: provider.defaultBackendUrl ?? "") }
        } catch { self.error = error.localizedDescription }
        isLoading = false
    }

    func loadStatus(provider: String, backendURL: String = "") async {
        do {
            let query = backendURL.isEmpty ? [] : [URLQueryItem(name: "backend_url", value: backendURL)]
            let status: LLMCredentialStatus = try await APIClient.shared.request("/llm/credentials/\(provider)", query: query)
            statuses[provider] = status
        } catch { self.error = error.localizedDescription }
    }

    func save(provider: String, key: String, backendURL: String) async throws {
        let status: LLMCredentialStatus = try await APIClient.shared.request("/llm/credentials/\(provider)", method: "PUT", body: LLMCredentialInput(apiKey: key, backendUrl: backendURL))
        statuses[provider] = status
    }

    func delete(provider: String, backendURL: String) async throws {
        let query = backendURL.isEmpty ? [] : [URLQueryItem(name: "backend_url", value: backendURL)]
        let _: DeletedResponse = try await APIClient.shared.request("/llm/credentials/\(provider)", method: "DELETE", query: query)
        await loadStatus(provider: provider, backendURL: backendURL)
    }

    func discover(provider: String, backendURL: String) async {
        do {
            let result: AIModelDiscoveryResponse = try await APIClient.shared.request("/analysis/ai/models", method: "POST", body: AIModelDiscoveryInput(provider: provider, backendUrl: backendURL.nilIfBlank))
            discoveredModels[provider] = result.models
        } catch { self.error = error.localizedDescription }
    }
}

struct AISettingsView: View {
    @StateObject private var model = AISettingsViewModel()
    @State private var editingProvider: AIProvider?

    var body: some View {
        Group {
            if model.isLoading && model.response == nil { LoadingStateView(label: "正在读取 AI 供应商") }
            else if let response = model.response {
                List {
                    Section {
                        ForEach(response.providers) { provider in
                            Button { editingProvider = provider } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 4) {
                                        Text(provider.name).foregroundStyle(.primary)
                                        Text(model.statuses[provider.id]?.configured == true ? "密钥已配置" : "密钥未配置").font(.caption).foregroundStyle(model.statuses[provider.id]?.configured == true ? AppTheme.positive : .secondary)
                                    }
                                    Spacer()
                                    Image(systemName: model.statuses[provider.id]?.configured == true ? "checkmark.circle.fill" : "chevron.right")
                                        .foregroundStyle(model.statuses[provider.id]?.configured == true ? AppTheme.positive : Color.secondary.opacity(0.55))
                                }
                            }
                        }
                    } footer: {
                        Text("密钥由 Go 后端加密保存；iOS 只能查看配置状态，无法读取原始值。")
                    }
                    if let error = model.error { Section { Text(error).foregroundStyle(AppTheme.negative) } }
                }
            } else if let error = model.error { ErrorStateView(message: error) { Task { await model.load() } } }
        }
        .navigationTitle("AI 模型与密钥")
        .navigationBarTitleDisplayMode(.inline)
        .task { await model.load() }
        .sheet(item: $editingProvider) { CredentialEditor(model: model, provider: $0) }
    }
}

private struct CredentialEditor: View {
    @ObservedObject var model: AISettingsViewModel
    let provider: AIProvider
    @Environment(\.dismiss) private var dismiss
    @State private var apiKey = ""
    @State private var backendURL: String
    @State private var isSaving = false
    @State private var confirmsDelete = false
    @State private var error: String?

    init(model: AISettingsViewModel, provider: AIProvider) {
        self.model = model; self.provider = provider
        _backendURL = State(initialValue: provider.defaultBackendUrl ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("凭据") {
                    SecureField("新的 API Key", text: $apiKey).textContentType(.password)
                    if provider.customModels ?? false { TextField("HTTPS 网关地址", text: $backendURL).keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled() }
                    if let status = model.statuses[provider.id] {
                        LabeledContent("状态", value: status.configured ? "已配置" : "未配置")
                        LabeledContent("加密存储", value: status.storageReady ? "可用" : "不可用")
                        if !status.backendHost.isEmpty { LabeledContent("网关", value: status.backendHost) }
                    }
                }
                Section("模型发现") {
                    Button("读取可用模型") { Task { await model.discover(provider: provider.id, backendURL: backendURL) } }
                    if let models = model.discoveredModels[provider.id] {
                        ForEach(models, id: \.self) { Text($0).font(.subheadline.monospaced()) }
                    }
                }
                if model.statuses[provider.id]?.configured == true {
                    Section { Button("删除服务端密钥", role: .destructive) { confirmsDelete = true } }
                }
                if let error { Section { Text(error).foregroundStyle(AppTheme.negative) } }
            }
            .navigationTitle(provider.name).navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("保存") { Task { await save() } }.disabled(apiKey.isEmpty || isSaving) }
            }
            .confirmationDialog("删除 \(provider.name) 的服务端密钥？", isPresented: $confirmsDelete, titleVisibility: .visible) {
                Button("删除", role: .destructive) { Task { await delete() } }
            } message: { Text("删除后相关 AI 分析会失去该凭据。") }
        }
    }

    private func save() async {
        isSaving = true; defer { isSaving = false }
        do { try await model.save(provider: provider.id, key: apiKey, backendURL: backendURL); dismiss() }
        catch { self.error = error.localizedDescription }
    }
    private func delete() async {
        do { try await model.delete(provider: provider.id, backendURL: backendURL); dismiss() }
        catch { self.error = error.localizedDescription }
    }
}
