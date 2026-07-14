import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { BleephubBuildFooter } from "../components/Shell.js";

describe("BleephubBuildFooter", () => {
  it("identifies the build and publication state on shared page chrome", () => {
    render(<BleephubBuildFooter />);

    expect(screen.getByTestId("bleephub-build-footer")).toHaveTextContent("Bleephub development");
    expect(screen.getByText("Published not yet published")).toBeInTheDocument();
  });
});
