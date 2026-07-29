import { NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

const publicRoutes = ["/signin", "/signup", "/"];

export default async function proxy(req: NextRequest) {
  const path = req.nextUrl.pathname;
  const isProtectedRoute = path.startsWith("/drive");
  const isPublicRoute = publicRoutes.includes(path);

  const refreshToken = (await cookies()).get("__Secure-Auth")?.value;
  const isRefreshTokenSet = refreshToken != null;

  // Redirect to /signin if the user is not authenticated
  if (isProtectedRoute && !refreshToken) {
    console.log(`User is not authenticated, redirecting to /signin`);
    return NextResponse.redirect(new URL("/signin", req.nextUrl));
  }

  // Redirect to /drive if the user is authenticated
  if (
    isPublicRoute &&
    isRefreshTokenSet &&
    !req.nextUrl.pathname.startsWith("/drive")
  ) {
    console.log("User is authenticated, redirecting to drive");
    return NextResponse.redirect(new URL("/drive", req.nextUrl));
  }

  return NextResponse.next();
}

// Routes Proxy should not run on
export const config = {
  matcher: ["/((?!api|_next/static|_next/image|.*\\.png$).*)"],
};
