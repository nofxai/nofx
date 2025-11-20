package logger

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLatestRecordsWithFilter(t *testing.T) {
	// 创建临时目录
	tmpDir, err := ioutil.TempDir("", "decision_logger_filter_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger := &DecisionLogger{
		logDir: tmpDir,
	}

	// 创建测试数据：10条记录，其中只有3条有操作
	testRecords := []struct {
		cycle      int
		hasActions bool
		timestamp  time.Time
	}{
		{1, true, time.Now().Add(-9 * time.Hour)},  // 有操作
		{2, false, time.Now().Add(-8 * time.Hour)}, // 无操作
		{3, false, time.Now().Add(-7 * time.Hour)}, // 无操作
		{4, true, time.Now().Add(-6 * time.Hour)},  // 有操作
		{5, false, time.Now().Add(-5 * time.Hour)}, // 无操作
		{6, false, time.Now().Add(-4 * time.Hour)}, // 无操作
		{7, false, time.Now().Add(-3 * time.Hour)}, // 无操作
		{8, true, time.Now().Add(-2 * time.Hour)},  // 有操作
		{9, false, time.Now().Add(-1 * time.Hour)}, // 无操作
		{10, false, time.Now()},                    // 无操作
	}

	for _, tr := range testRecords {
		record := &DecisionRecord{
			Timestamp:   tr.timestamp,
			CycleNumber: tr.cycle,
			Success:     true,
		}

		if tr.hasActions {
			record.Decisions = []DecisionAction{
				{Action: "open_long", Symbol: "BTC", Price: 50000},
			}
		} else {
			record.Decisions = []DecisionAction{}
		}

		// 写入文件
		data, _ := json.Marshal(record)
		// 使用正确的文件名格式
		filename := filepath.Join(tmpDir, tr.timestamp.Format("decision_20060102_150405_cycle")+string(rune(tr.cycle+48))+".json")
		ioutil.WriteFile(filename, data, 0644)
	}

	t.Run("GetLatestRecords without filter returns all records", func(t *testing.T) {
		records, err := logger.GetLatestRecordsWithFilter(10, false)
		require.NoError(t, err)
		assert.Equal(t, 10, len(records), "Should return all 10 records")
	})

	t.Run("GetLatestRecords with filter returns only records with actions", func(t *testing.T) {
		records, err := logger.GetLatestRecordsWithFilter(10, true)
		require.NoError(t, err)
		assert.Equal(t, 3, len(records), "Should return only 3 records with actions")

		// 验证返回的都是有操作的记录
		for _, record := range records {
			assert.True(t, len(record.Decisions) > 0, "All records should have actions")
		}
	})

	t.Run("GetLatestRecords with filter respects limit", func(t *testing.T) {
		records, err := logger.GetLatestRecordsWithFilter(2, true)
		require.NoError(t, err)
		assert.Equal(t, 2, len(records), "Should return only 2 records even though 3 have actions")

		// 验证返回的是有操作的记录
		for _, record := range records {
			assert.True(t, len(record.Decisions) > 0, "All records should have actions")
		}
	})

	t.Run("GetLatestRecords with filter when no actions exist", func(t *testing.T) {
		// 创建一个只有无操作记录的目录
		tmpDir2, err := ioutil.TempDir("", "decision_logger_filter_test_no_actions")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir2)

		logger2 := &DecisionLogger{logDir: tmpDir2}

		// 写入2条无操作记录
		for i := 1; i <= 2; i++ {
			ts := time.Now().Add(time.Duration(-i) * time.Hour)
			record := &DecisionRecord{
				Timestamp:   ts,
				CycleNumber: i,
				Decisions:   []DecisionAction{},
			}
			data, _ := json.Marshal(record)
			filename := filepath.Join(tmpDir2, ts.Format("decision_20060102_150405_cycle")+string(rune(i+48))+".json")
			ioutil.WriteFile(filename, data, 0644)
		}

		records, err := logger2.GetLatestRecordsWithFilter(10, true)
		require.NoError(t, err)
		assert.Equal(t, 0, len(records), "Should return empty slice when no records have actions")
	})

	t.Run("GetLatestRecords with filter excludes hold and wait actions", func(t *testing.T) {
		tmpDir3, err := ioutil.TempDir("", "decision_logger_filter_test_hold_wait")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir3)

		logger3 := &DecisionLogger{logDir: tmpDir3}

		// 创建测试记录：hold, wait, 实际操作
		testCases := []struct {
			cycle  int
			action string
		}{
			{1, "hold"},       // 应该被过滤掉
			{2, "wait"},       // 应该被过滤掉
			{3, "HOLD"},       // 大写也应该被过滤掉
			{4, "WAIT"},       // 大写也应该被过滤掉
			{5, "open_long"},  // 应该保留
			{6, "close_long"}, // 应该保留
		}

		for _, tc := range testCases {
			ts := time.Now().Add(time.Duration(-tc.cycle) * time.Hour)
			record := &DecisionRecord{
				Timestamp:   ts,
				CycleNumber: tc.cycle,
				Decisions: []DecisionAction{
					{Action: tc.action, Symbol: "BTC", Price: 50000},
				},
			}
			data, _ := json.Marshal(record)
			filename := filepath.Join(tmpDir3, ts.Format("decision_20060102_150405_cycle")+string(rune(tc.cycle+48))+".json")
			ioutil.WriteFile(filename, data, 0644)
		}

		records, err := logger3.GetLatestRecordsWithFilter(10, true)
		require.NoError(t, err)
		assert.Equal(t, 2, len(records), "Should return only 2 records (open_long and close_long)")

		// 验证返回的记录都不是 hold 或 wait
		for _, record := range records {
			for _, decision := range record.Decisions {
				action := decision.Action
				assert.NotEqual(t, "hold", action, "Should not contain 'hold' action")
				assert.NotEqual(t, "wait", action, "Should not contain 'wait' action")
				assert.NotEqual(t, "HOLD", action, "Should not contain 'HOLD' action")
				assert.NotEqual(t, "WAIT", action, "Should not contain 'WAIT' action")
			}
		}
	})
}
