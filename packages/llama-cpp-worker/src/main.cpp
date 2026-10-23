#include "llama.h"
#include "gguf.h"
#include <nlohmann/json.hpp>
#include <atomic>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <mutex>
#include <memory>
#include <set>
#include <stdexcept>
#include <string>
#include <thread>
#include <vector>

using json = nlohmann::json;
namespace fs = std::filesystem;
constexpr size_t max_frame = 16 * 1024 * 1024;

class Worker {
    std::mutex output_mutex;
    std::mutex state_mutex;
    std::thread computation;
    std::atomic<bool> cancelled{false};
    std::atomic<bool> busy{false};
    std::string active;
    llama_model *model = nullptr;
    llama_context *context = nullptr;
    std::string handle;
    size_t frame_limit = max_frame;
    bool initialized = false;

    void send(const std::string &id, json fields) {
        fields["version"] = 1;
        fields["type"] = fields.value("type", "response");
        fields["requestId"] = id;
        const auto data = fields.dump();
        if (data.size() + 1 > frame_limit) {
            throw std::runtime_error("worker frame limit exceeded");
        }
        std::lock_guard<std::mutex> lock(output_mutex);
        std::cout << data << '\n' << std::flush;
        if (!std::cout) {
            cancelled = true;
            throw std::runtime_error("protocol output closed");
        }
    }

    static json read_json(const fs::path &path) {
        std::ifstream source(path);
        if (!source) {
            throw std::runtime_error("cannot read " + path.string());
        }
        return json::parse(source);
    }

    void probe(const json &p) {
        if (p.at("backend") != "llamaCpp" || p.at("format") != "gguf") {
            throw std::runtime_error("llama.cpp requires a GGUF resource");
        }
        std::ifstream file(p.at("weightsPath").get<std::string>(), std::ios::binary);
        char header[8]{};
        file.read(header, 8);
        if (!file || std::string(header, 4) != "GGUF" || (header[4] != 2 && header[4] != 3)) {
            throw std::runtime_error("invalid GGUF file or version");
        }
        if (!fs::is_regular_file(fs::path(p.at("codecDirectory").get<std::string>()) / "tokenizer.json")) {
            throw std::runtime_error("tokenizer.json is required");
        }
        const auto path = p.at("weightsPath").get<std::string>();
        std::unique_ptr<gguf_context, decltype(&gguf_free)> metadata(
            gguf_init_from_file(path.c_str(), {true, nullptr}), gguf_free);
        if (!metadata || gguf_get_n_tensors(metadata.get()) == 0) {
            throw std::runtime_error("GGUF metadata or tensor table is invalid");
        }
        const auto file_size = fs::file_size(path);
        for (int64_t i = 0; i < gguf_get_n_tensors(metadata.get()); ++i) {
            const auto offset = gguf_get_data_offset(metadata.get()) + gguf_get_tensor_offset(metadata.get(), i);
            if (offset > file_size || gguf_get_tensor_size(metadata.get(), i) > file_size - offset) {
                throw std::runtime_error("truncated GGUF tensor data");
            }
        }
        // The pinned loader validates architecture, split files and vocabulary
        // without allocating tensor weights or an inference context.
        auto options = llama_model_default_params();
        options.vocab_only = true;
        options.n_gpu_layers = 0;
        std::unique_ptr<llama_model, decltype(&llama_model_free)> vocabulary(
            llama_model_load_from_file(path.c_str(), options), llama_model_free);
        if (!vocabulary) {
            throw std::runtime_error("GGUF architecture, shards or vocabulary are unsupported");
        }
        validate_vocabulary(p, vocabulary.get());
    }

    void validate_vocabulary(const json &p, const llama_model *loaded) {
        const auto tokenizer = read_json(fs::path(p.at("codecDirectory").get<std::string>()) / "tokenizer.json");
        const auto *vocab = llama_model_get_vocab(loaded);
        // The first supported families use BPE token strings with identical IDs.
        if (tokenizer.at("model").at("type") != "BPE") {
            throw std::runtime_error("this worker currently validates BPE tokenizers only");
        }
        auto check = [&](const std::string &piece, int id) {
            if (id < 0 || id >= llama_vocab_n_tokens(vocab) || piece != llama_vocab_get_text(vocab, id)) {
                throw std::runtime_error("GGUF/tokenizer token ID mapping mismatch at " + std::to_string(id));
            }
        };
        for (const auto &[piece, id] : tokenizer.at("model").at("vocab").items()) {
            check(piece, id.get<int>());
        }
        for (const auto &token : tokenizer.at("added_tokens")) {
            check(token.at("content").get<std::string>(), token.at("id").get<int>());
        }
    }

    void unload() {
        if (context) {
            llama_free(context);
            context = nullptr;
        }
        if (model) {
            llama_model_free(model);
            model = nullptr;
        }
        handle.clear();
    }

    static bool abort_compute(void *opaque) {
        return static_cast<Worker *>(opaque)->cancelled.load();
    }

    void operation(const std::string &id, const std::string &method, const json &p) {
        uint64_t sequence = 0;
        try {
            json result;
            if (method == "probeModel") {
                try {
                    probe(p);
                    result = {{"compatible", true}};
                } catch (const std::exception &e) {
                    result = {{"compatible", false}, {"reason", e.what()}};
                }
            } else if (method == "loadModel") {
                probe(p);
                unload();
                auto parameters = llama_model_default_params();
#ifdef __APPLE__
                parameters.n_gpu_layers = 99;
#endif
                model = llama_model_load_from_file(p.at("weightsPath").get<std::string>().c_str(), parameters);
                if (!model) {
                    throw std::runtime_error("GGUF model load failed");
                }
                validate_vocabulary(p, model);
                auto options = llama_context_default_params();
                options.n_ctx = std::min<uint32_t>(8192, llama_model_n_ctx_train(model));
                options.n_batch = 256;
                options.n_ubatch = 256;
                options.abort_callback = abort_compute;
                options.abort_callback_data = this;
                context = llama_init_from_model(model, options);
                if (!context) {
                    throw std::runtime_error("inference context allocation failed");
                }
                llama_token token = 0;
                if (llama_decode(context, llama_batch_get_one(&token, 1)) != 0) {
                    throw std::runtime_error("model readiness evaluation failed");
                }
                llama_memory_clear(llama_get_memory(context), true);
                handle = "model-" + id;
                result = {{"modelHandle", handle}, {"contextWindow", llama_n_ctx(context)},
                          {"vocabSize", llama_vocab_n_tokens(llama_model_get_vocab(model))},
                          {"tokenizerSha256", p.at("tokenizerSha256")}};
            } else if (method == "unloadModel") {
                if (p.at("modelHandle") != handle || !model) {
                    throw std::runtime_error("unknown model handle");
                }
                unload();
                result = json::object();
            } else if (method == "generate") {
                if (!context || p.at("modelHandle") != handle) {
                    throw std::runtime_error("unknown model handle");
                }
                auto ids = p.at("tokenIds").get<std::vector<llama_token>>();
                const int64_t budget = p.at("maxOutputTokens");
                if (ids.empty() || budget <= 0 || ids.size() + budget > llama_n_ctx(context)) {
                    throw std::runtime_error("prompt/output budget exceeds context");
                }
                const auto *vocab = llama_model_get_vocab(model);
                for (const auto token : ids) {
                    if (token < 0 || token >= llama_vocab_n_tokens(vocab)) {
                        throw std::runtime_error("input token out of range");
                    }
                }
                const auto stop_ids = p.at("stopTokenIds").get<std::set<llama_token>>();
                llama_memory_clear(llama_get_memory(context), true);
                int64_t count = 0;
                std::string reason = "maxTokens";
                for (size_t offset = 0; offset < ids.size() && !cancelled; offset += 256) {
                    const int size = static_cast<int>(std::min<size_t>(256, ids.size() - offset));
                    if (llama_decode(context, llama_batch_get_one(ids.data() + offset, size)) != 0 && !cancelled) {
                        throw std::runtime_error("prompt prefill failed");
                    }
                }
                auto *sampler = llama_sampler_chain_init(llama_sampler_chain_default_params());
                llama_sampler_chain_add(sampler, llama_sampler_init_top_k(40));
                llama_sampler_chain_add(sampler, llama_sampler_init_top_p(0.95f, 1));
                llama_sampler_chain_add(sampler, llama_sampler_init_temp(0.7f));
                llama_sampler_chain_add(sampler, llama_sampler_init_dist(LLAMA_DEFAULT_SEED));
                try {
                    while (count < budget && !cancelled) {
                        auto token = llama_sampler_sample(sampler, context, -1);
                        ++count;
                        send(id, {{"type", "event"}, {"sequence", ++sequence}, {"event", "tokens"},
                                  {"data", {{"tokenIds", json::array({token})}}}});
                        if (stop_ids.count(token) || llama_vocab_is_eog(vocab, token)) {
                            reason = "eos";
                            break;
                        }
                        if (count < budget && llama_decode(context, llama_batch_get_one(&token, 1)) != 0 && !cancelled) {
                            throw std::runtime_error("token evaluation failed");
                        }
                    }
                } catch (...) {
                    llama_sampler_free(sampler);
                    llama_memory_clear(llama_get_memory(context), true);
                    throw;
                }
                llama_sampler_free(sampler);
                llama_memory_clear(llama_get_memory(context), true);
                if (cancelled) {
                    reason = "cancelled";
                }
                result = {{"finishReason", reason}, {"usage", {{"inputTokens", ids.size()}, {"outputTokens", count}}}};
            } else {
                throw std::runtime_error("unknown operation " + method);
            }
            json response = {{"result", result}};
            if (method == "generate") {
                response["sequence"] = ++sequence;
            }
            {
                std::lock_guard<std::mutex> lock(state_mutex);
                active.clear();
                busy = false;
            }
            send(id, response);
        } catch (const std::exception &e) {
            if (method == "loadModel") {
                unload();
            }
            json error = {{"error", {{"code", "worker_error"}, {"message", e.what()}}}};
            if (method == "generate") {
                error["sequence"] = ++sequence;
            }
            {
                std::lock_guard<std::mutex> lock(state_mutex);
                active.clear();
                busy = false;
            }
            send(id, error);
        }
        std::lock_guard<std::mutex> lock(state_mutex);
        if (active == id) {
            active.clear();
            busy = false;
        }
    }

public:
    ~Worker() {
        cancelled = true;
        if (computation.joinable()) {
            computation.join();
        }
        unload();
    }
    void serve() {
        std::string line;
        // Bound input before allocating a full untrusted line.
        while (true) {
            line.clear();
            char ch;
            while (std::cin.get(ch) && ch != '\n') {
                if (line.size() >= frame_limit) {
                    throw std::runtime_error("input frame too large");
                }
                line.push_back(ch);
            }
            if (!std::cin) {
                if (!line.empty()) {
                    throw std::runtime_error("truncated NDJSON frame");
                }
                return;
            }
            const auto frame = json::parse(line);
            if (frame.at("version") != 1 || frame.at("type") != "request") {
                throw std::runtime_error("invalid request envelope");
            }
            const std::string id = frame.at("requestId");
            const std::string method = frame.at("method");
            const auto p = frame.value("params", json::object());
            if (id.empty()) {
                throw std::runtime_error("request ID is required");
            }
            if (method == "initialize") {
                if (initialized || p.at("protocolVersion") != 1) {
                    throw std::runtime_error("invalid initialization");
                }
                frame_limit = std::min(max_frame, p.at("maxFrameBytes").get<size_t>());
                if (frame_limit == 0) {
                    throw std::runtime_error("invalid frame limit");
                }
                initialized = true;
                send(id, {{"result", {{"protocolVersion", 1}, {"backend", "llamaCpp"},
                         {"engineVersion", "e3797b184d428c7a019362612b76971f60af6670"}, {"maxFrameBytes", frame_limit},
                         {"maxInputTokens", 8192}, {"maxOutputTokens", 8192},
                         {"maxConcurrentGenerations", 1}, {"supportsJsonSchema", false}}}});
            } else if (!initialized) {
                throw std::runtime_error("initialize must be called first");
            } else if (method == "cancel") {
                std::lock_guard<std::mutex> lock(state_mutex);
                const bool accepted = p.at("requestId") == active;
                if (accepted) {
                    cancelled = true;
                }
                send(id, {{"result", {{"accepted", accepted}}}});
            } else if (method == "shutdown") {
                cancelled = true;
                send(id, {{"result", json::object()}});
                return;
            } else {
                if (busy) {
                    send(id, {{"error", {{"code", "worker_busy"}, {"message", "one operation at a time"}}}});
                    continue;
                }
                if (computation.joinable()) {
                    computation.join();
                }
                {
                    std::lock_guard<std::mutex> lock(state_mutex);
                    active = id;
                    cancelled = false;
                    busy = true;
                }
                computation = std::thread([this, id, method, p] { operation(id, method, p); });
            }
        }
    }
};

int main() {
    llama_log_set([](ggml_log_level, const char *text, void *) { std::cerr << text; }, nullptr);
    llama_backend_init();
    int status = 0;
    try {
        Worker worker;
        worker.serve();
    } catch (const std::exception &e) {
        std::cerr << "llama-cpp-worker: " << e.what() << '\n';
        status = 1;
    }
    llama_backend_free();
    return status;
}
