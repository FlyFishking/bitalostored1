// Copyright 2019-2024 Xu Ruibo (hustxurb@163.com) and Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

type DashboardModel struct {
	Token     string `json:"token"`
	StartTime string `json:"start_time"`
	AdminAddr string `json:"admin_addr"`
	HostPort  string `json:"hostport"`

	BackupAddr     string `json:"backup_addr"`
	BackupHostPort string `json:"backup_hostport"`

	ProductName string `json:"product_name"`

	ReadCrossCloud bool `json:"read_cross_cloud"`

	Pid int    `json:"pid"`
	Pwd string `json:"pwd"`
	Sys string `json:"sys"`

	CgroupConfig string `json:"cgroup_config"`
}

func (t *DashboardModel) Encode() []byte {
	return jsonEncode(t)
}
