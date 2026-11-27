import unittest

from qnext_certify_microbench import WORKLOADS, compare, parse


def sample(ns=100, memory=100, allocations=10):
    return {name: [{"ns/op": ns, "B/op": memory, "allocs/op": allocations} for _ in range(5)]
            for name in WORKLOADS}


class CertificationTests(unittest.TestCase):
    def test_parse_only_reviewed_benchmark_lines(self):
        name = next(iter(WORKLOADS))
        self.assertEqual(parse(f"{name}-4  123  99.5 ns/op  100 B/op  2 allocs/op\n"),
                         {name: [{"ns/op": 99.5, "B/op": 100.0, "allocs/op": 2.0}]})
        self.assertEqual(parse("BenchmarkUnrelated-4  1  5 ns/op  0 B/op  0 allocs/op"), {})

    def test_pass_requires_gain_and_no_regression(self):
        self.assertEqual(compare(sample(), sample(80))["status"], "PASS")
        self.assertEqual(compare(sample(), sample())["status"], "INCONCLUSIVE")
        self.assertEqual(compare(sample(), sample(80, 120))["status"], "FAIL")

    def test_missing_samples_and_nonfinite_fail_closed(self):
        self.assertEqual(compare(sample(), {})["status"], "INCONCLUSIVE")
        self.assertEqual(compare(sample(), sample(float("nan")))["status"], "INCONCLUSIVE")
        self.assertEqual(compare(sample(memory=0), sample(80, 1))["status"], "FAIL")

    def test_median_ignores_one_outlier(self):
        after = sample(80)
        after[next(iter(WORKLOADS))][0]["ns/op"] = 100000
        self.assertEqual(compare(sample(), after)["status"], "PASS")


if __name__ == "__main__":
    unittest.main()
