// SPDX-License-Identifier: MIT
// Test-only fixture generator. Never embedded, installed or released.
#include <CreateBarcode.h>
#include <WriteBarcode.h>
#include <ImageView.h>
#include <iostream>
#include <stdexcept>
#include <string>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif
int main(int argc, char** argv) {
#ifdef _WIN32
    _setmode(_fileno(stdout), _O_BINARY);
#endif
    try {
        if (argc != 3) throw std::runtime_error("expected micro-qr|rmqr payload");
        std::string format = argv[1];
        if (format != "micro-qr" && format != "rmqr") throw std::runtime_error("unsupported test format");
        auto code = ZXing::CreateBarcodeFromText(argv[2], ZXing::CreatorOptions(
            format == "micro-qr" ? ZXing::BarcodeFormat::MicroQRCode : ZXing::BarcodeFormat::RMQRCode));
        auto image = ZXing::WriteBarcodeToImage(code, ZXing::WriterOptions().scale(8).addQuietZones(true));
        std::cout << "P5\n" << image.width() << ' ' << image.height() << "\n255\n";
        for (int y = 0; y < image.height(); ++y)
            std::cout.write(reinterpret_cast<const char*>(image.data()) + y * image.rowStride(), image.width());
        return std::cout ? 0 : 2;
    } catch (const std::exception& e) { std::cerr << e.what() << '\n'; return 2; }
}
