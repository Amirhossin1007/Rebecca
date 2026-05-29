import { clearAdminsCache } from "contexts/AdminsContext";
import { clearDashboardCache } from "contexts/DashboardContext";
import { clearHostsCache } from "contexts/HostsContext";
import { clearServicesCache } from "contexts/ServicesContext";
import { removeAuthToken } from "utils/authStorage";
import { queryClient } from "utils/react-query";

export const clearClientSession = () => {
	removeAuthToken();
	queryClient.clear();
	clearAdminsCache();
	clearDashboardCache();
	clearServicesCache();
	clearHostsCache();
};
