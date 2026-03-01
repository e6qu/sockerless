import { describe, it, expect, afterEach } from "vitest";
import { render, fireEvent, cleanup } from "@testing-library/react";
import { createColumnHelper } from "@tanstack/react-table";
import { DataTable } from "../components/DataTable.js";

afterEach(cleanup);

interface Row {
  name: string;
  age: number;
}

const col = createColumnHelper<Row>();

const columns = [
  col.accessor("name", { header: "Name" }),
  col.accessor("age", { header: "Age" }),
];

const data: Row[] = [
  { name: "Alice", age: 30 },
  { name: "Bob", age: 25 },
  { name: "Charlie", age: 35 },
];

describe("DataTable", () => {
  it("renders rows from data", () => {
    const { container } = render(<DataTable data={data} columns={columns} />);

    const rows = container.querySelectorAll("tbody tr");
    expect(rows).toHaveLength(3);

    const names = Array.from(rows).map(
      (row) => row.querySelectorAll("td")[0].textContent,
    );
    expect(names).toContain("Alice");
    expect(names).toContain("Bob");
    expect(names).toContain("Charlie");
  });

  it("sorts by column header click", () => {
    const { container } = render(<DataTable data={data} columns={columns} />);

    const headers = container.querySelectorAll("th");
    const ageHeader = headers[1];

    // First click sorts (desc for numeric columns by default)
    fireEvent.click(ageHeader);
    let rows = container.querySelectorAll("tbody tr");
    let ageValues = Array.from(rows).map(
      (row) => row.querySelectorAll("td")[1].textContent,
    );
    expect(ageValues).toEqual(["35", "30", "25"]);

    // Second click reverses sort
    fireEvent.click(ageHeader);
    rows = container.querySelectorAll("tbody tr");
    ageValues = Array.from(rows).map(
      (row) => row.querySelectorAll("td")[1].textContent,
    );
    expect(ageValues).toEqual(["25", "30", "35"]);
  });

  it("filters with global search input", () => {
    const { container } = render(
      <DataTable data={data} columns={columns} filterPlaceholder="Search..." />,
    );

    const input = container.querySelector("input")!;
    fireEvent.change(input, { target: { value: "Ali" } });

    const rows = container.querySelectorAll("tbody tr");
    expect(rows).toHaveLength(1);
    expect(rows[0].querySelectorAll("td")[0].textContent).toBe("Alice");
  });
});
