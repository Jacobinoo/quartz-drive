import { z } from "zod";

const envSchema = z.object({
  NEXT_PUBLIC_API_URL: z.url(),
  NEXT_PUBLIC_SERVER_PUBLIC_KEY: z.string().min(1),
  NEXT_PUBLIC_S3_ENDPOINT: z.string().min(1),
  NEXT_PUBLIC_CAPTCHA_SITE_KEY: z.string().min(1)
});

const parsedEnv = envSchema.safeParse({
  NEXT_PUBLIC_API_URL: process.env.NEXT_PUBLIC_API_URL,
  NEXT_PUBLIC_SERVER_PUBLIC_KEY: process.env.NEXT_PUBLIC_SERVER_PUBLIC_KEY,
  NEXT_PUBLIC_S3_ENDPOINT: process.env.NEXT_PUBLIC_S3_ENDPOINT,
  NEXT_PUBLIC_CAPTCHA_SITE_KEY: process.env.NEXT_PUBLIC_CAPTCHA_SITE_KEY,
});

if (!parsedEnv.success) {
  console.error(z.treeifyError(parsedEnv.error))
  throw new Error("Invalid environment variables");
}

export const config = {
  apiUrl: parsedEnv.data.NEXT_PUBLIC_API_URL,
  serverPublicKey: parsedEnv.data.NEXT_PUBLIC_SERVER_PUBLIC_KEY,
  s3Endpoint: parsedEnv.data.NEXT_PUBLIC_S3_ENDPOINT,
  captchaSitekey: parsedEnv.data.NEXT_PUBLIC_CAPTCHA_SITE_KEY
};
