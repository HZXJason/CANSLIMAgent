import SwiftUI

enum AppTheme {
    static let accent = Color(red: 0.15, green: 0.48, blue: 0.98)
    static let positive = Color(red: 0.10, green: 0.67, blue: 0.43)
    static let negative = Color(red: 0.91, green: 0.26, blue: 0.30)
    static let warning = Color(red: 0.95, green: 0.60, blue: 0.12)
    static let cardCorner: CGFloat = 18
}

struct ResearchCard<Content: View>: View {
    let title: String
    var subtitle: String? = nil
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(.headline)
                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle).font(.caption).foregroundStyle(.secondary)
                }
            }
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: AppTheme.cardCorner, style: .continuous))
    }
}
struct LoadingStateView: View {
    var label = "正在加载"
    var body: some View {
        HStack(spacing: 10) {
            ProgressView()
            Text(label).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, minHeight: 88)
    }
}

struct ErrorStateView: View {
    let message: String
    var retry: (() -> Void)?

    var body: some View {
        ContentUnavailableView {
            Label("暂时无法加载", systemImage: "exclamationmark.triangle")
        } description: {
            Text(message)
        } actions: {
            if let retry { Button("重试", action: retry) }
        }
    }
}

struct PressFeedback: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
            .opacity(configuration.isPressed ? 0.86 : 1)
            .animation(.easeOut(duration: 0.12), value: configuration.isPressed)
    }
}
