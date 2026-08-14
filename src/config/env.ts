import { z } from "zod";

const envSchema = z.object({
  NEXT_PUBLIC_API_URL: z.url(),
  NEXT_PUBLIC_SERVER_PUBLIC_KEY: z.string().min(1),
});

const parsedEnv = envSchema.safeParse({
  NEXT_PUBLIC_API_URL: process.env.NEXT_PUBLIC_API_URL,
  NEXT_PUBLIC_SERVER_PUBLIC_KEY: process.env.NEXT_PUBLIC_SERVER_PUBLIC_KEY,
});

if (!parsedEnv.success) {
  console.error(z.treeifyError(parsedEnv.error))
  throw new Error("Invalid environment variables");
}

export const config = {
  apiUrl: parsedEnv.data.NEXT_PUBLIC_API_URL,
  serverPublicKey: parsedEnv.data.NEXT_PUBLIC_SERVER_PUBLIC_KEY
};
