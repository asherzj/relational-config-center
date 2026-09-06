import { useState, type ReactNode } from "react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";

/** Exercise the same data-router navigation/blocking semantics as production. */
export function TestRouter({ children, initialEntries }: { children: ReactNode; initialEntries: string[] }) {
  const [router] = useState(() => createMemoryRouter([{ path: "*", element: children }], { initialEntries }));
  return <RouterProvider router={router} />;
}
