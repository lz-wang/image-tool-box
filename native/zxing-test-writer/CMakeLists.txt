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
    GIT_TAG v3.1.1
    GIT_SHALLOW TRUE
    GIT_SUBMODULES zint
    GIT_SUBMODULES_RECURSE FALSE
    SOURCE_SUBDIR core)
  FetchContent_MakeAvailable(zxing)
endif()
add_executable(zxing-test-writer main.cpp)
target_link_libraries(zxing-test-writer PRIVATE ZXing::ZXing)
