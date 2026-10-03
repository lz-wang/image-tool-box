cmake_minimum_required(VERSION 3.20)
project(itb_zxing_test_writer LANGUAGES CXX C)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(ZXING_READERS OFF CACHE BOOL "" FORCE)
set(ZXING_WRITERS NEW CACHE STRING "" FORCE)
set(ZXING_EXPERIMENTAL_API ON CACHE BOOL "" FORCE)
set(ZXING_ENABLE_QRCODE ON CACHE BOOL "" FORCE)
foreach(format 1D AZTEC DATAMATRIX MAXICODE PDF417)
  set(ZXING_ENABLE_${format} OFF CACHE BOOL "" FORCE)
endforeach()
if(ZXING_SOURCE_DIR)
  add_subdirectory("${ZXING_SOURCE_DIR}/core" zxing)
else()
  include(FetchContent)
  FetchContent_Declare(zxing
    GIT_REPOSITORY https://github.com/zxing-cpp/zxing-cpp.git
    # ZXing-C++ v3.1.1, pinned to the immutable release commit.
    GIT_TAG 287c85df6f961c8efbfb5ffd736cd9457b8b890e
    GIT_SHALLOW FALSE
    GIT_SUBMODULES zint
    GIT_SUBMODULES_RECURSE FALSE
    SOURCE_SUBDIR core)
  FetchContent_MakeAvailable(zxing)
  find_package(Git REQUIRED)
  execute_process(
    COMMAND "${GIT_EXECUTABLE}" -C "${zxing_SOURCE_DIR}/zint" rev-parse HEAD
    RESULT_VARIABLE zint_result
    OUTPUT_VARIABLE zint_revision
    OUTPUT_STRIP_TRAILING_WHITESPACE)
  # Match the submodule revision recorded by the pinned ZXing release.
  if(NOT zint_result EQUAL 0 OR NOT zint_revision STREQUAL "55541e139e62b9209b71cd9b0ba9010cec28b1d9")
    message(FATAL_ERROR "Unexpected Zint revision: ${zint_revision}")
  endif()
endif()
add_executable(zxing-test-writer main.cpp)
target_link_libraries(zxing-test-writer PRIVATE ZXing::ZXing)
