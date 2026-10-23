cmake_minimum_required(VERSION 3.24)
project(agenty_llama_worker LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
set(BUILD_SHARED_LIBS OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_TESTS OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_EXAMPLES OFF CACHE BOOL "" FORCE)
set(LLAMA_BUILD_SERVER OFF CACHE BOOL "" FORCE)
set(GGML_NATIVE OFF CACHE BOOL "" FORCE)
set(GGML_METAL_EMBED_LIBRARY ON CACHE BOOL "" FORCE)
include(FetchContent)
FetchContent_Declare(llama
    URL https://codeload.github.com/ggml-org/llama.cpp/tar.gz/e3797b184d428c7a019362612b76971f60af6670
    URL_HASH SHA256=e1bb69742ec9b86da121a25247432a4c05b71e3c87f59e5dcaef67fe284a2200)
FetchContent_Declare(json
    URL https://codeload.github.com/nlohmann/json/tar.gz/65ee68451d8eb2b5f3a30b410476ab83deb3289b
    URL_HASH SHA256=13ef31d691947940a08909f8e0772f1d7d68e5da1678ee812a49c4bb0c996b2f)
FetchContent_MakeAvailable(llama json)
find_package(Threads REQUIRED)
add_executable(llama-cpp-worker src/main.cpp)
target_link_libraries(llama-cpp-worker PRIVATE llama nlohmann_json::nlohmann_json Threads::Threads)
install(TARGETS llama-cpp-worker RUNTIME DESTINATION bin)
