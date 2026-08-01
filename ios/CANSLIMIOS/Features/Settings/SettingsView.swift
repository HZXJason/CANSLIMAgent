import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var settings: SettingsStore
    @State private var baseURL = ""
    @State private var token = ""
    @State private var message: String?
    @State private var isChecking = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("API 地址", text: $baseURL)
                        .textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL)
                    SecureField(settings.apiToken.isEmpty ? "访问令牌（可选）" : "已安全保存；留空保持不变", text: $token)
                    Button("保存连接") { save() }
                    Button {
                        Task { await checkHealth() }
                    } label: {
                        HStack {
                            Text("测试连接")
                            Spacer()
                            if isChecking { ProgressView() }
                        }
                    }
                    .disabled(isChecking)
                    if let message { Text(message).font(.footnote).foregroundStyle(message.contains("成功") ? AppTheme.positive : AppTheme.negative) }
                } header: {
                    Text("连接")
                } footer: {
                    Text("模拟器可使用 127.0.0.1；真机请填写 Mac 的局域网主机名或 HTTPS 地址。令牌只保存在本机 Keychain。")
                }

                Section("市场") {
                    Picker("资产市场", selection: $settings.assetMarket) {
                        ForEach(AssetMarket.allCases) { Text($0.title).tag($0) }
                    }
                }

                Section("研究与账户") {
                    NavigationLink("AI 模型与密钥") { AISettingsView() }
                    NavigationLink("券商连接") { BrokerSettingsView() }
                }

                Section("关于") {
                    LabeledContent("版本", value: "1.0 (1)")
                    Text("研究决策支持工具，不会自动下单，也不构成投资建议。")
                        .font(.footnote).foregroundStyle(.secondary)
                }
            }
            .navigationTitle("设置")
            .onAppear { baseURL = settings.apiBaseURL }
        }
    }

    private func save() {
        settings.apiBaseURL = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            if !token.isEmpty { try settings.saveAPIToken(token); token = "" }
            message = "连接设置已保存"
        } catch { message = error.localizedDescription }
    }

    private func checkHealth() async {
        save()
        isChecking = true
        defer { isChecking = false }
        do {
            let _: [String: String] = try await APIClient.shared.request("/health")
            message = "连接成功"
        } catch { message = error.localizedDescription }
    }
}
