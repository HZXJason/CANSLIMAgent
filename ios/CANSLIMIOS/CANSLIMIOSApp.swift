import SwiftUI

@main
struct CANSLIMIOSApp: App {
    @StateObject private var settings = SettingsStore.shared

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(settings)
                .tint(AppTheme.accent)
        }
    }
}
