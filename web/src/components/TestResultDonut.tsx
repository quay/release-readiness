import { ChartDonut } from "@patternfly/react-charts/victory";

interface TestResultDonutProps {
  passed: number;
  failed: number;
  skipped: number;
  total: number;
}

export default function TestResultDonut({
  passed,
  failed,
  skipped,
  total,
}: TestResultDonutProps) {
  if (total === 0) {
    return <span>No test data</span>;
  }

  const passRate = Math.round((passed / total) * 100);

  return (
    <div style={{ height: "150px", width: "150px" }}>
      <ChartDonut
        constrainToVisibleArea
        data={[
          { x: "Passed", y: passed },
          { x: "Failed", y: failed },
          { x: "Skipped", y: skipped },
        ]}
        colorScale={["#3E8635", "#C9190B", "#6A6E73"]}
        title={`${passRate}%`}
        subTitle="pass rate"
        width={150}
        height={150}
        innerRadius={45}
        padding={{ top: 0, bottom: 0, left: 0, right: 0 }}
      />
    </div>
  );
}
