import Foundation
import SwiftUI

private struct Suggestion: Decodable, Sendable {
    let guess: String
    let candidates: Int
}

private struct Turn: Identifiable {
    let id = UUID()
    let guess: String
    let feedback: [Int]
}

private enum SolverClient {
    static func suggest(history: [String]) throws -> Suggestion {
        let appURL = Bundle.main.bundleURL
        let executableURL = appURL.appendingPathComponent("Contents/Resources/wordsolver")
        let process = Process()
        let output = Pipe()

        process.executableURL = executableURL
        process.arguments = ["-suggest", "-history", history.joined(separator: ",")]
        process.standardOutput = output
        process.standardError = output
        try process.run()

        let data = output.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        guard process.terminationStatus == 0 else {
            let message = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
            throw NSError(
                domain: "WordSolver",
                code: Int(process.terminationStatus),
                userInfo: [NSLocalizedDescriptionKey: message.isEmpty ? "Не удалось получить подсказку." : message]
            )
        }
        return try JSONDecoder().decode(Suggestion.self, from: data)
    }
}

private struct FeedbackTile: View {
    let letter: String
    let value: Int
    var action: (() -> Void)?

    private var tileColor: Color {
        switch value {
        case 1: .white
        case 2: Color(red: 1, green: 1, blue: 0)
        default: Color(red: 0.34, green: 0.36, blue: 0.38)
        }
    }

    private var letterColor: Color {
        value == 1 || value == 2 ? .black : .white
    }

    var body: some View {
        Group {
            if let action {
                Button(action: action) {
                    tile
                }
                .buttonStyle(.plain)
                .help("Нажмите, чтобы изменить цвет")
            } else {
                tile
            }
        }
    }

    private var tile: some View {
        Text(letter.uppercased())
            .font(.system(size: 25, weight: .bold, design: .rounded))
            .foregroundStyle(letterColor)
            .frame(width: 50, height: 50)
            .background(tileColor, in: RoundedRectangle(cornerRadius: 8))
            .overlay {
                RoundedRectangle(cornerRadius: 8)
                    .stroke(value == 1 ? Color.gray.opacity(0.7) : .clear, lineWidth: 1.5)
            }
    }
}

@main
struct WordSolverApp: App {
    var body: some Scene {
        WindowGroup {
            SolverView()
                .frame(minWidth: 440, minHeight: 570)
        }
        .windowResizability(.contentSize)
    }
}

private struct SolverView: View {
    @State private var turns: [Turn] = []
    @State private var feedback = Array(repeating: 0, count: 5)
    @State private var suggestion: Suggestion?
    @State private var isLoading = false
    @State private var isSolved = false
    @State private var errorMessage: String?

    private var history: [String] {
        turns.map { turn in
            "\(turn.guess):\(turn.feedback.map(String.init).joined())"
        }
    }

    var body: some View {
        VStack(spacing: 22) {
            VStack(spacing: 5) {
                Text("Решатель «5 букв»")
                    .font(.system(size: 28, weight: .bold, design: .rounded))
                Text("Отмечайте результат каждой попытки")
                    .foregroundStyle(.secondary)
            }
            .padding(.top, 28)

            if !turns.isEmpty {
                VStack(alignment: .leading, spacing: 10) {
                    ForEach(Array(turns.enumerated()), id: \.element.id) { index, turn in
                        HStack(spacing: 8) {
                            Text("\(index + 1).")
                                .foregroundStyle(.secondary)
                                .frame(width: 25, alignment: .trailing)
                            ForEach(0..<5, id: \.self) { position in
                                FeedbackTile(letter: String(Array(turn.guess)[position]), value: turn.feedback[position])
                            }
                        }
                    }
                }
            }

            if isSolved {
                Label("Отлично! Слово угадано.", systemImage: "checkmark.circle.fill")
                    .font(.title3.weight(.semibold))
                    .foregroundStyle(.green)
            } else if turns.count >= 6 {
                Label("Попытки закончились.", systemImage: "exclamationmark.circle")
                    .font(.title3.weight(.semibold))
                    .foregroundStyle(.secondary)
            } else if let suggestion {
                VStack(spacing: 12) {
                    Text("Попытка \(turns.count + 1) из 6")
                        .font(.headline)
                    Text("Предлагаемое слово")
                        .foregroundStyle(.secondary)
                    HStack(spacing: 8) {
                        ForEach(0..<5, id: \.self) { position in
                            FeedbackTile(letter: String(Array(suggestion.guess)[position]), value: 0)
                        }
                    }
                    Text("Осталось возможных слов: \(suggestion.candidates)")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                }

                VStack(spacing: 10) {
                    Text("Нажимайте на буквы, чтобы менять цвет")
                        .font(.callout)
                        .foregroundStyle(.secondary)
                    HStack(spacing: 8) {
                        ForEach(0..<5, id: \.self) { position in
                            FeedbackTile(
                                letter: String(Array(suggestion.guess)[position]),
                                value: feedback[position]
                            ) {
                                feedback[position] = (feedback[position] + 1) % 3
                            }
                        }
                    }
                    HStack(spacing: 14) {
                        legend(color: Color(red: 0.34, green: 0.36, blue: 0.38), title: "Нет")
                        legend(color: .white, title: "Есть, не на месте")
                        legend(color: Color(red: 1, green: 1, blue: 0), title: "На месте")
                    }
                }

                Button(action: submitFeedback) {
                    if isLoading {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                    } else {
                        Text("Продолжить")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)
                .disabled(isLoading)
            } else if isLoading {
                ProgressView("Подбираю слово…")
                    .padding(.vertical, 30)
            }

            if let errorMessage {
                Text(errorMessage)
                    .font(.callout)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
                Button("Повторить", action: requestSuggestion)
                    .disabled(isLoading)
            }

            Spacer(minLength: 0)

            Button("Новая игра", action: startNewGame)
                .buttonStyle(.link)
                .padding(.bottom, 20)
        }
        .padding(.horizontal, 28)
        .onAppear(perform: requestSuggestion)
    }

    private func legend(color: Color, title: String) -> some View {
        HStack(spacing: 5) {
            Circle()
                .fill(color)
                .overlay(Circle().stroke(title == "Есть, не на месте" ? .gray.opacity(0.5) : .clear, lineWidth: 1))
                .frame(width: 9, height: 9)
            Text(title).font(.caption).foregroundStyle(.secondary)
        }
    }

    private func submitFeedback() {
        guard let suggestion else { return }
        let played = suggestion.guess
        let result = feedback
        turns.append(Turn(guess: played, feedback: result))
        feedback = Array(repeating: 0, count: 5)

        if result.allSatisfy({ $0 == 2 }) {
            isSolved = true
            self.suggestion = nil
        } else {
            self.suggestion = nil
            requestSuggestion()
        }
    }

    private func startNewGame() {
        turns = []
        feedback = Array(repeating: 0, count: 5)
        suggestion = nil
        isSolved = false
        errorMessage = nil
        requestSuggestion()
    }

    private func requestSuggestion() {
        guard !isLoading else { return }
        isLoading = true
        errorMessage = nil
        let currentHistory = history
        Task {
            do {
                let next = try await Task.detached(priority: .userInitiated) {
                    try SolverClient.suggest(history: currentHistory)
                }.value
                suggestion = next
            } catch {
                errorMessage = error.localizedDescription
            }
            isLoading = false
        }
    }
}
