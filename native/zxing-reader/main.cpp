// SPDX-License-Identifier: MIT
// Private ITBZ v1: magic[4], version[1], big-endian u32 width/height/stride/mask,
// then stride*height Gray8 bytes. No image parsing, paths or writer API.
#include <ReadBarcode.h>
#include <Barcode.h>
#include <BarcodeFormat.h>
#include <ReaderOptions.h>
#include <ImageView.h>
#include <array>
#include <cstdint>
#include <iostream>
#include <stdexcept>
#include <string>
#include <vector>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif

static uint32_t read32(const unsigned char* p) {
    return uint32_t(p[0]) << 24 | uint32_t(p[1]) << 16 | uint32_t(p[2]) << 8 | p[3];
}

static std::string quote(const std::string& value) {
    const char* hex = "0123456789abcdef";
    std::string out = "\"";
    for (unsigned char c : value) {
        if (c == '"' || c == '\\') { out += '\\'; out += char(c); }
        else if (c < 32) { out += "\\u00"; out += hex[c >> 4]; out += hex[c & 15]; }
        else out += char(c);
    }
    return out + '"';
}

int main() {
#ifdef _WIN32
    _setmode(_fileno(stdin), _O_BINARY);
#endif
    try {
        std::array<unsigned char, 21> header{};
        if (!std::cin.read(reinterpret_cast<char*>(header.data()), header.size()))
            throw std::runtime_error("truncated header");
        if (std::string(reinterpret_cast<char*>(header.data()), 4) != "ITBZ" || header[4] != 1)
            throw std::runtime_error("invalid magic or version");
        uint32_t w = read32(&header[5]), h = read32(&header[9]);
        uint32_t stride = read32(&header[13]), mask = read32(&header[17]);
        if (!w || !h || w > 32768 || h > 32768 || stride != w ||
            uint64_t(stride) * h > 64 * 1024 * 1024 || !mask || (mask & ~127u))
            throw std::runtime_error("invalid dimensions or formats");
        std::vector<uint8_t> pixels(size_t(stride) * h);
        if (!std::cin.read(reinterpret_cast<char*>(pixels.data()), pixels.size()))
            throw std::runtime_error("truncated pixels");
        if (std::cin.peek() != EOF) throw std::runtime_error("trailing data");
        using F = ZXing::BarcodeFormat;
        constexpr std::array<F, 7> formats{F::QRCodeModel2, F::MicroQRCode, F::RMQRCode,
            F::Code128, F::Code39Std, F::EAN13, F::EAN8};
        constexpr std::array<const char*, 7> names{"QRCodeModel2", "MicroQRCode", "RMQRCode",
            "Code128", "Code39", "EAN13", "EAN8"};
        std::vector<F> selected;
        for (size_t i = 0; i < formats.size(); ++i) if (mask & (1u << i)) selected.push_back(formats[i]);
        auto opts = ZXing::ReaderOptions().setFormats(ZXing::BarcodeFormats(std::move(selected))).setTryHarder(true)
            .setTryRotate(true).setTryInvert(true).setMaxNumberOfSymbols(255)
            .setValidateOptionalChecksum(false).setTextMode(ZXing::TextMode::Plain);
        auto codes = ZXing::ReadBarcodes(ZXing::ImageView(pixels.data(), int(w), int(h),
            ZXing::ImageFormat::Lum, int(stride)), opts);
        std::cout << "{\"version\":1,\"codes\":[";
        bool first = true;
        for (const auto& code : codes) {
            if (!code.isValid()) continue;
            size_t index = 0;
            while (index < formats.size() && code.format() != formats[index]) ++index;
            if (index == formats.size()) continue;
            if (!first) std::cout << ',';
            first = false;
            std::cout << "{\"format\":" << quote(names[index]) << ",\"text\":" << quote(code.text()) << ",\"points\":[";
            bool firstPoint = true;
            for (const auto& point : code.position()) {
                if (!firstPoint) std::cout << ',';
                firstPoint = false;
                std::cout << "{\"x\":" << point.x << ",\"y\":" << point.y << '}';
            }
            std::cout << "]}";
        }
        std::cout << "]}\n";
        if (!std::cout) throw std::runtime_error("output failed");
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 2;
    }
}
