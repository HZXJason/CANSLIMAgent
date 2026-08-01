import Foundation

enum APIError: LocalizedError, Equatable {
    case invalidBaseURL
    case invalidResponse
    case message(String)

    var errorDescription: String? {
        switch self {
        case .invalidBaseURL: "API 地址无效"
        case .invalidResponse: "服务器返回了无法识别的响应"
        case let .message(message): message
        }
    }
}

@MainActor
final class APIClient {
    static let shared = APIClient()

    private let session: URLSession
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder

    init(session: URLSession = .shared) {
        self.session = session
        decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
    }

    func request<Response: Decodable, Body: Encodable>(
        _ path: String,
        method: String = "GET",
        query: [URLQueryItem] = [],
        body: Body? = Optional<EmptyBody>.none
    ) async throws -> Response {
        var request = try makeRequest(path: path, method: method, query: query)
        if let body {
            request.httpBody = try encoder.encode(body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (data, response) = try await session.data(for: request)
        try validate(response: response, data: data)
        do {
            return try decoder.decode(APIEnvelope<Response>.self, from: data).data
        } catch {
            throw APIError.message("响应解析失败：\(error.localizedDescription)")
        }
    }

    func request<Response: Decodable>(
        _ path: String,
        method: String = "GET",
        query: [URLQueryItem] = []
    ) async throws -> Response {
        try await request(path, method: method, query: query, body: Optional<EmptyBody>.none)
    }

    func raw(_ path: String, query: [URLQueryItem] = []) async throws -> (Data, URLResponse) {
        let request = try makeRequest(path: path, method: "GET", query: query)
        let (data, response) = try await session.data(for: request)
        try validate(response: response, data: data)
        return (data, response)
    }

    func stream<Body: Encodable>(
        _ path: String,
        method: String = "GET",
        query: [URLQueryItem] = [],
        body: Body? = Optional<EmptyBody>.none
    ) async throws -> (URLSession.AsyncBytes, URLResponse) {
        var request = try makeRequest(path: path, method: method, query: query)
        request.timeoutInterval = 60 * 60
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        if let body {
            request.httpBody = try encoder.encode(body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let result = try await session.bytes(for: request)
        try validate(response: result.1, data: nil)
        return result
    }

    private func makeRequest(path: String, method: String, query: [URLQueryItem]) throws -> URLRequest {
        let rawBase = SettingsStore.shared.apiBaseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        guard var components = URLComponents(string: rawBase.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + path) else {
            throw APIError.invalidBaseURL
        }
        if !query.isEmpty { components.queryItems = query }
        guard let url = components.url else { throw APIError.invalidBaseURL }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.timeoutInterval = 30
        let token = SettingsStore.shared.apiToken
        if !token.isEmpty { request.setValue(token, forHTTPHeaderField: "X-Internal-API-Token") }
        return request
    }

    private func validate(response: URLResponse, data: Data?) throws {
        guard let http = response as? HTTPURLResponse else { throw APIError.invalidResponse }
        guard (200..<300).contains(http.statusCode) else {
            if let data,
               let failure = try? decoder.decode(APIFailure.self, from: data),
               let message = failure.error ?? failure.message {
                throw APIError.message(message)
            }
            throw APIError.message("请求失败（HTTP \(http.statusCode)）")
        }
    }
}

private struct EmptyBody: Encodable {}
