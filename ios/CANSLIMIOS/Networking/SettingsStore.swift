import Foundation
import Security

@MainActor
final class SettingsStore: ObservableObject {
    static let shared = SettingsStore()

    @Published var apiBaseURL: String {
        didSet { UserDefaults.standard.set(apiBaseURL, forKey: Keys.apiBaseURL) }
    }
    @Published var assetMarket: AssetMarket {
        didSet { UserDefaults.standard.set(assetMarket.rawValue, forKey: Keys.assetMarket) }
    }

    var apiToken: String { KeychainStore.read() ?? "" }

    private enum Keys {
        static let apiBaseURL = "canslim.apiBaseURL"
        static let assetMarket = "canslim.assetMarket"
    }

    private init() {
        apiBaseURL = UserDefaults.standard.string(forKey: Keys.apiBaseURL) ?? "http://127.0.0.1:8080/api/v1"
        assetMarket = AssetMarket(rawValue: UserDefaults.standard.string(forKey: Keys.assetMarket) ?? "US") ?? .us
    }

    func saveAPIToken(_ token: String) throws {
        if token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            KeychainStore.delete()
        } else {
            try KeychainStore.save(token.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        objectWillChange.send()
    }
}
private enum KeychainStore {
    private static let service = "com.hzxjason.CANSLIMIOS"
    private static let account = "internal-api-token"

    static func save(_ value: String) throws {
        delete()
        let status = SecItemAdd([
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
            kSecAttrAccessible: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
            kSecValueData: Data(value.utf8)
        ] as CFDictionary, nil)
        guard status == errSecSuccess else { throw APIError.message("无法安全保存访问令牌（\(status)）") }
    }

    static func read() -> String? {
        var result: CFTypeRef?
        let status = SecItemCopyMatching([
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account,
            kSecReturnData: true,
            kSecMatchLimit: kSecMatchLimitOne
        ] as CFDictionary, &result)
        guard status == errSecSuccess, let data = result as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    static func delete() {
        SecItemDelete([
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: service,
            kSecAttrAccount: account
        ] as CFDictionary)
    }
}
