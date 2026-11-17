/** Send a local API request and return an explicit result for expected failures. */
export async function request<T>(
  path: string,
  body?: unknown,
  method = "POST",
  rawJSON = false,
): Promise<{ value: T; error: null } | { value: null; error: string }> {
  try {
    const response = await fetch(
      path,
      body === undefined
        ? { method: "GET" }
        : {
            method,
            headers: { "Content-Type": "application/json" },
            body: rawJSON ? String(body) : JSON.stringify(body),
          },
    );
    const data: unknown = await response.json();
    if (!response.ok) {
      const message =
        typeof data === "object" && data !== null && "error" in data
          ? String(data.error)
          : `Request failed (${response.status})`;
      return { value: null, error: message };
    }
    return { value: data as T, error: null };
  } catch (error) {
    return {
      value: null,
      error:
        error instanceof Error ? error.message : "The local server could not be reached.",
    };
  }
}
