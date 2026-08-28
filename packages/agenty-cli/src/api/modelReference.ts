import type { ModelDto, ModelRef } from "./types";

export type ModelIdentity = Pick<ModelDto, "providerCode" | "code">;

export function modelRefFromModel(model: ModelIdentity): ModelRef {
    return {
        providerCode: model.providerCode,
        modelCode: model.code,
    };
}

export function formatModelRef(ref: ModelRef): string {
    return `${ref.providerCode}/${ref.modelCode}`;
}

export function sameModelRef(left: ModelRef | undefined, right: ModelRef | undefined): boolean {
    return !!left && !!right && left.providerCode === right.providerCode && left.modelCode === right.modelCode;
}

export function findModelByRef(models: readonly ModelDto[], ref: ModelRef): ModelDto | undefined {
    return models.find((model) => model.providerCode === ref.providerCode && model.code === ref.modelCode);
}

/**
 * Resolve text entered at a CLI/UI boundary. Canonical provider/model
 * references take precedence over every shorthand so model IDs containing '/'
 * cannot shadow an explicitly scoped reference.
 */
export function resolveModelInput(models: readonly ModelDto[], input: string): ModelDto {
    const reference = input.trim();
    const lower = reference.toLowerCase();

    const canonicalMatches = models.filter((model) => formatModelRef(modelRefFromModel(model)) === reference);
    if (canonicalMatches.length === 1) {
        return canonicalMatches[0];
    }
    if (canonicalMatches.length > 1) {
        throw ambiguousModelReference(reference);
    }

    const codeMatches = models.filter((model) => model.code === reference);
    if (codeMatches.length === 1) {
        return codeMatches[0];
    }
    if (codeMatches.length > 1) {
        throw ambiguousModelReference(reference);
    }

    const nameMatches = models.filter((model) => model.name.toLowerCase() === lower);
    if (nameMatches.length === 1) {
        return nameMatches[0];
    }
    if (nameMatches.length > 1) {
        throw ambiguousModelReference(reference);
    }

    const providerNameMatches = models.filter((model) =>
        `${model.providerName}/${model.name}`.toLowerCase() === lower,
    );
    if (providerNameMatches.length === 1) {
        return providerNameMatches[0];
    }
    if (providerNameMatches.length > 1) {
        throw ambiguousModelReference(reference);
    }

    throw new Error(`model not found: ${reference}`);
}

function ambiguousModelReference(reference: string): Error {
    return new Error(`model reference is ambiguous: ${reference}; use <provider-code>/<model-code>`);
}
