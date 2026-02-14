import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Page, Masthead, MastheadMain, MastheadBrand } from "@patternfly/react-core";
import "@patternfly/react-core/dist/styles/base.css";

import ReleasesOverview from "./pages/ReleasesOverview";
import ReleaseDetail from "./pages/ReleaseDetail";
import SnapshotsList from "./pages/SnapshotsList";

function AppLayout({ children }: { children: React.ReactNode }) {
  const header = (
    <Masthead>
      <MastheadMain>
        <MastheadBrand>
          <a href="/" style={{ color: "inherit", textDecoration: "none", fontWeight: 600 }}>
            Release Readiness Dashboard
          </a>
        </MastheadBrand>
      </MastheadMain>
    </Masthead>
  );

  return <Page masthead={header}>{children}</Page>;
}

export default function App() {
  return (
    <BrowserRouter>
      <AppLayout>
        <Routes>
          <Route path="/" element={<ReleasesOverview />} />
          <Route path="/releases/:app" element={<ReleaseDetail />} />
          <Route path="/releases/:app/snapshots" element={<SnapshotsList />} />
        </Routes>
      </AppLayout>
    </BrowserRouter>
  );
}
