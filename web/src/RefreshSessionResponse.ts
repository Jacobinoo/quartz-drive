export type RefreshSessionResponse = {
    status: "ok" | "not_ok";
    csrfToken: string;
    token: string;
    wrappedAccountKeys: string;
};
