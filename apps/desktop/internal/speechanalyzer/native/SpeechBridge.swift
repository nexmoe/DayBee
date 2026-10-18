import AVFoundation
import Foundation
import Speech

private final class Request: @unchecked Sendable {
    private let lock = NSLock()
    private var response: String?
    private var task: Task<Void, Never>?

    func start(_ work: @escaping @Sendable () async throws -> [String: Any]) {
        lock.lock()
        task = Task {
            let value: [String: Any]
            do { value = try await work() }
            catch { value = ["error": "SpeechAnalyzer: \(error.localizedDescription)"] }
            let data = try? JSONSerialization.data(withJSONObject: value)
            complete(data.flatMap { String(data: $0, encoding: .utf8) } ?? "{\"error\":\"Invalid speech response\"}")
        }
        lock.unlock()
    }

    private func complete(_ value: String) {
        lock.lock()
        response = value
        task = nil
        lock.unlock()
    }

    func poll() -> UnsafeMutablePointer<CChar>? {
        lock.lock()
        defer { lock.unlock() }
        return response.flatMap { strdup($0) }
    }

    func cancel() {
        lock.lock()
        task?.cancel()
        lock.unlock()
    }
}

private func failure(_ message: String) -> NSError {
    NSError(domain: "DayBee.SpeechAnalyzer", code: 1, userInfo: [NSLocalizedDescriptionKey: message])
}

@available(macOS 26.0, *)
private func selectedLocale(_ language: String) async throws -> Locale {
    let candidates = language.isEmpty || language == "auto"
        ? Locale.preferredLanguages + [Locale.current.identifier] : [language]
    for candidate in candidates {
        if let locale = await SpeechTranscriber.supportedLocale(equivalentTo: Locale(identifier: candidate)) {
            return locale
        }
    }
    throw failure("The selected language is not supported. Choose another transcription language or model.")
}

// The converter yields bounded chunks on demand, including its final resampler
// tail. AnalyzerInput itself does not resample arbitrary input formats.
@available(macOS 26.0, *)
private final class PCMSource: @unchecked Sendable {
    let samples: [Float]
    let sourceFormat: AVAudioFormat
    let targetFormat: AVAudioFormat
    let converter: AVAudioConverter
    var offset = 0

    init(_ samples: [Float], rate: Int, format: AVAudioFormat) throws {
        guard let source = AVAudioFormat(commonFormat: .pcmFormatFloat32, sampleRate: Double(rate), channels: 1, interleaved: false),
              let converter = AVAudioConverter(from: source, to: format) else {
            throw failure("Unable to convert audio for transcription.")
        }
        self.samples = samples
        sourceFormat = source
        targetFormat = format
        self.converter = converter
    }

    func next() throws -> AnalyzerInput? {
        try Task.checkCancellation()
        guard let output = AVAudioPCMBuffer(pcmFormat: targetFormat, frameCapacity: 4096) else {
            throw failure("Unable to allocate transcription audio.")
        }
        var error: NSError?
        let status = converter.convert(to: output, error: &error) { count, state in
            guard self.offset < self.samples.count else {
                state.pointee = .endOfStream
                return nil
            }
            let n = min(Int(count), self.samples.count - self.offset)
            guard let buffer = AVAudioPCMBuffer(pcmFormat: self.sourceFormat, frameCapacity: AVAudioFrameCount(n)),
                  let channel = buffer.floatChannelData?[0] else {
                state.pointee = .noDataNow
                return nil
            }
            self.samples.withUnsafeBufferPointer { pcm in
                channel.update(from: pcm.baseAddress! + self.offset, count: n)
            }
            self.offset += n
            buffer.frameLength = AVAudioFrameCount(n)
            state.pointee = .haveData
            return buffer
        }
        if let error { throw error }
        if output.frameLength > 0 { return AnalyzerInput(buffer: output) }
        if status == .endOfStream { return nil }
        throw failure("Audio conversion produced no samples.")
    }
}

@available(macOS 26.0, *)
private func perform(_ operation: String, language: String, samples: [Float], rate: Int) async throws -> [String: Any] {
    let locale = try await selectedLocale(language)
    let transcriber = SpeechTranscriber(locale: locale, preset: .transcription)
    if operation == "prepare" {
        try await AssetInventory.reserve(locale: locale)
        if let installation = try await AssetInventory.assetInstallationRequest(supporting: [transcriber]) {
            try await installation.downloadAndInstall()
        }
    }
    let installed = await AssetInventory.status(forModules: [transcriber]) == .installed
    if operation != "decode" { return ["ready": installed] }
    guard installed else { throw failure("Prepare the system speech model before transcribing.") }
    guard let format = await SpeechAnalyzer.bestAvailableAudioFormat(compatibleWith: [transcriber]) else {
        throw failure("No compatible transcription audio format is available.")
    }
    let source = try PCMSource(samples, rate: rate, format: format)
    let inputs = AsyncThrowingStream<AnalyzerInput, Error>(unfolding: { try source.next() })
    let analyzer = SpeechAnalyzer(modules: [transcriber])
    let results = Task {
        var text = ""
        for try await result in transcriber.results {
            text += String(result.text.characters)
        }
        return text.trimmingCharacters(in: .whitespacesAndNewlines)
    }
    return try await withTaskCancellationHandler {
        do {
            if let end = try await analyzer.analyzeSequence(inputs) {
                try await analyzer.finalizeAndFinish(through: end)
            } else {
                await analyzer.cancelAndFinishNow()
            }
            return ["text": try await results.value, "language": locale.language.languageCode?.identifier ?? locale.identifier]
        } catch {
            await analyzer.cancelAndFinishNow()
            results.cancel()
            throw error
        }
    } onCancel: {
        Task { await analyzer.cancelAndFinishNow() }
    }
}

@_cdecl("daybee_speech_available")
public func speechAvailable() -> Int32 {
    if #available(macOS 26.0, *) { return SpeechTranscriber.isAvailable ? 1 : 0 }
    return 0
}

@_cdecl("daybee_speech_start")
public func speechStart(_ operation: UnsafePointer<CChar>, _ language: UnsafePointer<CChar>,
                        _ pcm: UnsafePointer<Float>?, _ count: Int64, _ rate: Int32) -> UnsafeMutableRawPointer {
    let request = Request()
    let op = String(cString: operation), locale = String(cString: language)
    let samples = pcm.map { Array(UnsafeBufferPointer(start: $0, count: Int(count))) } ?? []
    request.start {
        if #available(macOS 26.0, *) {
            return try await perform(op, language: locale, samples: samples, rate: Int(rate))
        }
        throw failure("macOS 26 or later is required.")
    }
    return Unmanaged.passRetained(request).toOpaque()
}

@_cdecl("daybee_speech_poll")
public func speechPoll(_ handle: UnsafeMutableRawPointer) -> UnsafeMutablePointer<CChar>? {
    Unmanaged<Request>.fromOpaque(handle).takeUnretainedValue().poll()
}

@_cdecl("daybee_speech_cancel")
public func speechCancel(_ handle: UnsafeMutableRawPointer) {
    Unmanaged<Request>.fromOpaque(handle).takeUnretainedValue().cancel()
}

@_cdecl("daybee_speech_release")
public func speechRelease(_ handle: UnsafeMutableRawPointer) {
    Unmanaged<Request>.fromOpaque(handle).release()
}
