
type Base64String = string;

interface KDFParams {
    kdfAlg: number;
    kdfOpsLimit: number;
    kdfMemLimit: number;
}

export type {
    Base64String,
    KDFParams
}