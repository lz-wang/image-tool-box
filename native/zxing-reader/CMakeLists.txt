cmake_minimum_required(VERSION 3.20)
project(itb_zxing_reader LANGUAGES CXX C)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")

set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(ZXING_READERS ON CACHE BOOL "" FORCE)
set(ZXING_WRITERS OFF CACHE STRING "" FORCE)
set(ZXING_ENABLE_1D ON CACHE BOOL "" FORCE)
set(ZXING_ENABLE_QRCODE ON CACHE BOOL "" FORCE)
foreach(format AZTEC DATAMATRIX MAXICODE PDF417)
  set(ZXING_ENABLE_${format} OFF CACHE BOOL "" FORCE)
endforeach()

# Offline builds may point at a verified v3.1.1 source tree.
if(ZXING_SOURCE_DIR)
  add_subdirectory("${ZXING_SOURCE_DIR}/core" zxing)
else()
  include(FetchContent)
  FetchContent_Declare(zxing
    URL https://github.com/zxing-cpp/zxing-cpp/archive/refs/tags/v3.1.1.tar.gz
    URL_HASH SHA256=7286b1e6ade66fe82b7c8208b4595deeb55d6486b410834fdc65702f46650542
    SOURCE_SUBDIR core)
  FetchContent_MakeAvailable(zxing)
endif()

add_executable(zxing-reader main.cpp)
target_link_libraries(zxing-reader PRIVATE ZXing::ZXing)
if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
  # GCC's static C++ runtime can still add ld-linux to NEEDED on arm64.
  # Full static linking preserves the existing shared-library allowlist.
  target_link_options(zxing-reader PRIVATE -static -static-libstdc++ -static-libgcc)
endif()
install(TARGETS zxing-reader RUNTIME DESTINATION . COMPONENT ITBReader)
