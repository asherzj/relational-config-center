import { createContext, useContext, useState, type ReactNode } from "react";
import { createMemoryRouter, RouterProvider } from "react-router-dom";

const RouteChildren = createContext<ReactNode>(null);
function CurrentChildren() { return useContext(RouteChildren); }

/** Exercise the same data-router navigation/blocking semantics as production. */
export function TestRouter({ children, initialEntries }: { children: ReactNode; initialEntries: string[] }) {
  const [router] = useState(() => createMemoryRouter([{ path: "*", element: <CurrentChildren /> }], { initialEntries }));
  return <RouteChildren.Provider value={children}><RouterProvider router={router} /></RouteChildren.Provider>;
}
